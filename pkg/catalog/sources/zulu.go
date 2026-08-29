package sources

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

const zuluDefaultBase = "https://api.azul.com"

type zulu struct {
	base
	client *http.Client
}

// NewZulu 构造 Azul Zulu 源适配器。
func NewZulu(client *http.Client) *zulu {
	return &zulu{base: base{base: zuluDefaultBase}, client: client}
}

func (s *zulu) ID() string   { return "zulu" }
func (s *zulu) Name() string { return "Azul Zulu" }

type zuluPackage struct {
	// Zulu API 的 java_version 是数组，如 [21,0,12,1]，需自行拼成 "21.0.12.1"。
	JavaVersion []int  `json:"java_version"`
	Name        string `json:"name"`
	SHA256      string `json:"sha256_hash"`
	DownloadURL string `json:"download_url"`
}

func (s *zulu) Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error) {
	if !p.Supported() {
		return nil, fmt.Errorf("Zulu 暂不支持架构 %s", p.Arch)
	}
	osName := map[string]string{"linux": "linux", "darwin": "macosx", "windows": "windows"}[p.OS]
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[p.Arch]
	archiveType := "tar.gz"
	if p.IsWindows() {
		archiveType = "zip"
	}

	majors := JavaMajors(ctx, s.client)
	var mu sync.Mutex
	var rels []*domain.Release
	err := parallelFetch(ctx, majors, 4, func(major int) error {
		url := fmt.Sprintf(
			"%s/metadata/v1/zulu/packages/?java_version=%d&os=%s&arch=%s&archive_type=%s&java_package_type=jdk&javafx_bundled=false&crac_supported=false&release_status=ga&latest=true&availability_types=CA",
			s.join(""), major, osName, arch, archiveType)
		var pkgs []zuluPackage
		if err := getJSON(ctx, s.client, url, &pkgs); err != nil {
			return nil // 单个 major 失败不阻断
		}
		if len(pkgs) == 0 {
			return nil
		}
		it := pkgs[0]
		// 数组 [21,0,12,1] -> "21.0.12.1"
		parts := make([]string, 0, len(it.JavaVersion))
		for _, n := range it.JavaVersion {
			parts = append(parts, fmt.Sprint(n))
		}
		display := strings.Join(parts, ".") + "-zul"
		if display == "-zul" {
			return nil
		}
		mu.Lock()
		rels = append(rels, &domain.Release{
			Version: display,
			Stable:  true,
			Artifact: &domain.Artifact{
				URL:          it.DownloadURL,
				SHA256:       it.SHA256,
				Ext:          archiveType,
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
	return rels, nil
}
