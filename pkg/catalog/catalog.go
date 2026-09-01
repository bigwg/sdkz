// Package catalog 负责候选元数据：内置定义 + 远程源适配。
package catalog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"sdkz/pkg/catalog/sources"
	"sdkz/pkg/config"
	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

// Source 是远程版本清单适配器。
type Source interface {
	ID() string
	Name() string
	Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error)
}

// Manager 管理全部候选与远程源。
type Manager struct {
	cfg      *config.Config
	client   *http.Client
	sources  map[string]Source
	baseURLs map[string]string
	// Warn 用于输出非致命告警（如回退缓存），CLI 接 stderr，GUI 可收集。
	Warn func(format string, args ...any)
}

// githubTokenTransport 在请求发往 api.github.com 时自动附带 Authorization 头，
// 以绕过 GitHub 匿名 API 的限流（60 次/小时）。仅当 GITHUB_TOKEN 环境变量非空时启用。
type githubTokenTransport struct {
	base  http.RoundTripper
	token string
}

func (t *githubTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL != nil && req.Header.Get("Authorization") == "" {
		h := req.URL.Host
		if h == "api.github.com" || h == "ghproxy.net" || strings.HasSuffix(h, "ghproxy.net") {
			req = req.Clone(req.Context())
			req.Header.Set("Authorization", "Bearer "+t.token)
		}
	}
	return t.base.RoundTrip(req)
}

// NewManager 构造 catalog 管理器并注册全部内置源。
func NewManager(cfg *config.Config) *Manager {
	httpClient := &http.Client{Timeout: 60 * time.Second}
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		httpClient.Transport = &githubTokenTransport{base: http.DefaultTransport, token: tok}
	}
	m := &Manager{
		cfg:      cfg,
		client:   httpClient,
		sources:  make(map[string]Source),
		baseURLs: make(map[string]string),
		Warn:     func(string, ...any) {},
	}
	m.register(sources.NewAdoptium(m.client))
	m.register(sources.NewZulu(m.client))
	m.register(sources.NewGraalVM(m.client))
	m.register(sources.NewGolang(m.client))
	m.register(sources.NewNodeJS(m.client))
	m.register(sources.NewMaven(m.client))
	m.register(sources.NewGradle(m.client))
	// 基于 GitHub Releases 的 JDK 发行版（各厂商命名差异大，使用通用适配器 + 各自解析规则）。
	m.register(sources.NewKonaSource(m.client))
	m.register(sources.NewDragonwellSource(m.client))
	m.register(sources.NewSAPMachineSource(m.client))
	return m
}

func (m *Manager) register(s Source) {
	if b, ok := m.baseURLs[s.ID()]; ok && b != "" {
		if setter, ok := s.(interface{ SetBaseURL(string) }); ok {
			setter.SetBaseURL(b)
		}
	}
	m.sources[s.ID()] = s
}

// SetBaseURL 覆盖某源的基础 URL（测试注入 / 特殊镜像）。
func (m *Manager) SetBaseURL(sourceID, base string) {
	m.baseURLs[sourceID] = base
	if s, ok := m.sources[sourceID]; ok {
		if setter, ok := s.(interface{ SetBaseURL(string) }); ok {
			setter.SetBaseURL(base)
		}
	}
}

func (m *Manager) Platform() platform.Platform { return platform.Detect() }

// BuiltinCandidates 返回内置候选定义。
func BuiltinCandidates() []*domain.Candidate { return builtinCandidates() }

// FindCandidate 按 id 查找内置候选。
func FindCandidate(id string) (*domain.Candidate, bool) {
	for _, c := range builtinCandidates() {
		if c.ID == id {
			return c, true
		}
	}
	return nil, false
}

// ListRemote 拉取（或从缓存读取）候选的全部版本清单。
// vendorID 为空时拉取全部发行版。
func (m *Manager) ListRemote(ctx context.Context, cand *domain.Candidate, vendorID string) ([]*domain.Release, error) {
	vendors := selectVendors(cand, vendorID)
	if len(vendors) == 0 {
		return nil, fmt.Errorf("未知发行版: %s", vendorID)
	}
	var all []*domain.Release
	var failed []string
	for _, v := range vendors {
		s, ok := m.sources[v.SourceID]
		if !ok {
			return nil, fmt.Errorf("候选 %s 的源 %q 未注册", cand.ID, v.SourceID)
		}
		rels, err := s.Fetch(ctx, m.Platform())
		if err != nil {
			// 无可用缓存：该发行版暂时不可用，但其余发行版仍应展示。
			failed = append(failed, v.Name)
			continue
		}
		for _, r := range rels {
			r.CandidateID = cand.ID
			r.VendorID = v.ID
			r.VendorName = v.Name
		}
		all = append(all, rels...)
	}
	// 全部发行版均失败才返回错误；部分失败仅告警。
	if len(all) == 0 && len(failed) > 0 {
		return nil, fmt.Errorf("%s 所有发行版获取失败: %s", cand.ID, strings.Join(failed, ", "))
	}
	if len(failed) > 0 {
		m.Warn("%s 以下发行版暂时不可用（已忽略）: %s", cand.ID, strings.Join(failed, ", "))
	}
	return all, nil
}

func selectVendors(cand *domain.Candidate, vendorID string) []*domain.Vendor {
	if vendorID != "" {
		if v, ok := cand.FindVendor(vendorID); ok {
			return []*domain.Vendor{v}
		}
		return nil
	}
	// 无 vendor 概念的候选：其 Vendors 仅含一个 official。
	vs := make([]*domain.Vendor, 0, len(cand.Vendors))
	for i := range cand.Vendors {
		vs = append(vs, &cand.Vendors[i])
	}
	return vs
}
