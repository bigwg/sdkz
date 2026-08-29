package sources

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

const adoptiumDefaultBase = "https://api.adoptium.net"

type adoptium struct {
	base
	client *http.Client
}

// NewAdoptium 构造 Eclipse Temurin 源适配器。
func NewAdoptium(client *http.Client) *adoptium {
	return &adoptium{base: base{base: adoptiumDefaultBase}, client: client}
}

func (s *adoptium) ID() string   { return "temurin" }
func (s *adoptium) Name() string { return "Eclipse Temurin" }

type adoptiumAsset struct {
	// Adoptium 的 version 字段为对象（含 major/minor/security/openjdk_version 等）。
	Version struct {
		OpenJDKVersion string `json:"openjdk_version"` // 如 "21.0.12.1+1-LTS"
	} `json:"version"`
	Binary struct {
		Package struct {
			Name     string `json:"name"`
			Link     string `json:"link"`
			Checksum string `json:"checksum"`
		} `json:"package"`
	} `json:"binary"`
}

func (s *adoptium) Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error) {
	if !p.Supported() {
		return nil, fmt.Errorf("Temurin 暂不支持架构 %s", p.Arch)
	}
	arch := "x64"
	if p.Arch == "arm64" {
		arch = "aarch64"
	}
	osName := p.OS
	if p.IsDarwin() {
		osName = "mac"
	}
	osName = strings.ToLower(osName)

	majors := JavaMajors(ctx, s.client)
	var mu sync.Mutex
	var rels []*domain.Release
	var failed int32
	err := parallelFetch(ctx, majors, 4, func(major int) error {
		url := fmt.Sprintf("%s/v3/assets/latest/%d/hotspot?architecture=%s&image_type=jdk&jvm_impl=hotspot&os=%s&vendor=eclipse&release_type=ga",
			s.join(""), major, arch, osName)
		var assets []adoptiumAsset
		if err := getJSON(ctx, s.client, url, &assets); err != nil {
			// 单个 major 拉取失败：记录但不中断整体（可能该架构无对应版本）。
			atomic.AddInt32(&failed, 1)
			return nil
		}
		if len(assets) == 0 {
			return nil
		}
		it := assets[0]
		display := normalizeAdoptiumVersion(it.Version.OpenJDKVersion)
		if display == "" {
			return nil
		}
		// 统一带发行版后缀，使标识符风格与 zulu/graal 一致（如 21.0.12.1+1-tem）。
		display += "-tem"
		ext := "tar.gz"
		if strings.HasSuffix(it.Binary.Package.Name, ".zip") {
			ext = "zip"
		}
		mu.Lock()
		rels = append(rels, &domain.Release{
			Version: display,
			Stable:  true,
			Artifact: &domain.Artifact{
				URL:          it.Binary.Package.Link,
				SHA256:       it.Binary.Package.Checksum,
				Ext:          ext,
				Strip:        1,
				ChecksumType: "sha256",
			},
		})
		mu.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 全部 major 均失败（如网络异常/API 变化）时返回错误，避免静默空列表。
	if len(rels) == 0 && failed > 0 {
		return nil, fmt.Errorf("Temurin 所有版本拉取失败（%d 个主版本）", failed)
	}
	return rels, nil
}

// normalizeAdoptiumVersion 将 "21.0.12.1+1-LTS" 规范为 "21.0.12.1+1"。
func normalizeAdoptiumVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// 去掉末尾的 -LTS / -ea / -ga 等发布标识。
	if i := strings.LastIndex(raw, "-"); i >= 0 {
		raw = raw[:i]
	}
	return raw
}
