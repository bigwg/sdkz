package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sdkz/pkg/config"
	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

// fakeSrc 可控的假源：err 非空时 Fetch 失败。
type fakeSrc struct {
	err  error
	rels []*domain.Release
}

func (f *fakeSrc) ID() string   { return "fake" }
func (f *fakeSrc) Name() string { return "Fake" }
func (f *fakeSrc) Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rels, nil
}

func newCacheTestManager(t *testing.T, src Source, warns *[]string) *Manager {
	t.Helper()
	m := &Manager{
		cfg:     &config.Config{Root: t.TempDir()},
		sources: map[string]Source{"fake": src},
		Warn: func(format string, args ...any) {
			if warns != nil {
				*warns = append(*warns, fmt.Sprintf(format, args...))
			}
		},
	}
	return m
}

func cacheTestCandidate() *domain.Candidate {
	return &domain.Candidate{ID: "test", Vendors: []domain.Vendor{{ID: "v", Name: "Vendor", SourceID: "fake"}}}
}

// 首次拉取写缓存；网络失败时新鲜缓存兜底且不告警。
func TestListRemoteFreshCacheSkipsNetwork(t *testing.T) {
	rels := []*domain.Release{{Version: "1.0.0", Stable: true}}
	src := &fakeSrc{rels: rels}
	var warns []string
	m := newCacheTestManager(t, src, &warns)
	cand := cacheTestCandidate()

	if _, err := m.ListRemote(context.Background(), cand, ""); err != nil {
		t.Fatalf("首次拉取失败: %v", err)
	}
	if _, err := os.Stat(m.CachePath("test", "v")); err != nil {
		t.Fatalf("缓存文件未写入: %v", err)
	}

	// 源改为失败：新鲜缓存应直接命中，不触网、不告警。
	src.err = errors.New("HTTP 403")
	got, err := m.ListRemote(context.Background(), cand, "")
	if err != nil {
		t.Fatalf("新鲜缓存兜底失败: %v", err)
	}
	if len(got) != 1 || got[0].Version != "1.0.0" {
		t.Fatalf("缓存返回内容不符: %+v", got)
	}
	if len(warns) != 0 {
		t.Fatalf("新鲜命中不应产生告警: %v", warns)
	}
}

// 过期缓存：网络失败时回退旧缓存并告警；网络正常时刷新缓存。
func TestListRemoteStaleCacheFallback(t *testing.T) {
	rels := []*domain.Release{{Version: "1.0.0", Stable: true}}
	src := &fakeSrc{rels: rels}
	var warns []string
	m := newCacheTestManager(t, src, &warns)
	cand := cacheTestCandidate()

	if _, err := m.ListRemote(context.Background(), cand, ""); err != nil {
		t.Fatalf("首次拉取失败: %v", err)
	}
	// 人为把缓存时间改到 TTL 之前。
	path := m.CachePath("test", "v")
	data, _ := os.ReadFile(path)
	var cf cacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		t.Fatal(err)
	}
	cf.Generated = time.Now().Add(-2 * MetadataTTL)
	fresh, _ := json.Marshal(cf)
	if err := os.WriteFile(path, fresh, 0o644); err != nil {
		t.Fatal(err)
	}

	// 源失败：应回退过期缓存并告警。
	src.err = errors.New("HTTP 403")
	got, err := m.ListRemote(context.Background(), cand, "")
	if err != nil {
		t.Fatalf("过期缓存回退失败: %v", err)
	}
	if len(got) != 1 || got[0].Version != "1.0.0" {
		t.Fatalf("回退内容不符: %+v", got)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "403") {
		t.Fatalf("应产生一次包含失败原因的告警: %v", warns)
	}

	// 源恢复：网络成功后缓存时间被刷新，且不再告警。
	src.err = nil
	warns = nil
	if _, err := m.ListRemote(context.Background(), cand, ""); err != nil {
		t.Fatalf("恢复拉取失败: %v", err)
	}
	data, _ = os.ReadFile(path)
	var cf2 cacheFile
	if err := json.Unmarshal(data, &cf2); err != nil {
		t.Fatal(err)
	}
	if time.Since(cf2.Generated) >= MetadataTTL {
		t.Fatalf("缓存未被刷新: %v", cf2.Generated)
	}
	if len(warns) != 0 {
		t.Fatalf("成功拉取不应告警: %v", warns)
	}
}

// 无缓存且源失败：维持原有"部分失败告警 / 全部失败报错"语义。
func TestListRemoteNoCacheFails(t *testing.T) {
	src := &fakeSrc{err: errors.New("boom")}
	var warns []string
	m := newCacheTestManager(t, src, &warns)

	if _, err := m.ListRemote(context.Background(), cacheTestCandidate(), ""); err == nil {
		t.Fatal("无缓存且源失败应返回错误")
	}
	if _, err := os.Stat(filepath.Join(m.cfg.Root, "metadata")); !os.IsNotExist(err) {
		t.Fatalf("失败时不应写缓存: %v", err)
	}
}
