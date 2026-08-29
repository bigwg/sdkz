package sources

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

const golangDefaultBase = "https://go.dev"

type golang struct {
	base
	client *http.Client
}

// NewGolang 构造 Go 官方源适配器。
func NewGolang(client *http.Client) *golang {
	return &golang{base: base{base: golangDefaultBase}, client: client}
}

func (s *golang) ID() string   { return "golang" }
func (s *golang) Name() string { return "Go Official" }

type goFile struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	SHA256   string `json:"sha256"`
	Kind     string `json:"kind"`
}

type goVersion struct {
	Version string   `json:"version"`
	Files   []goFile `json:"files"`
}

func (s *golang) Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error) {
	var versions []goVersion
	url := s.join("/dl/?mode=json&include=all")
	if err := getJSON(ctx, s.client, url, &versions); err != nil {
		return nil, fmt.Errorf("获取 Go 版本清单失败: %w", err)
	}

	collect := func(preRelease bool) []*domain.Release {
		var out []*domain.Release
		for _, gv := range versions {
			if domain.IsPreReleaseString(gv.Version) != preRelease {
				continue
			}
			for _, f := range gv.Files {
				if f.Kind != "archive" || f.OS != p.OS || f.Arch != p.Arch {
					continue
				}
				ext := "tar.gz"
				if strings.HasSuffix(f.Filename, ".zip") {
					ext = "zip"
				}
				out = append(out, &domain.Release{
					Version: gv.Version, // 如 go1.23.4
					Stable:  !preRelease,
					Artifact: &domain.Artifact{
						URL:          s.join("/dl/" + f.Filename),
						SHA256:       f.SHA256,
						Ext:          ext,
						Strip:        1,
						ChecksumType: "sha256",
					},
				})
				break
			}
		}
		return out
	}

	stable := collect(false)
	if len(stable) > 0 {
		return stable, nil
	}
	return collect(true), nil
}
