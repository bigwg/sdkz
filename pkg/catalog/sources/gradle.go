package sources

import (
	"context"
	"fmt"
	"net/http"

	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

const gradleDefaultBase = "https://services.gradle.org"

type gradle struct {
	base
	client *http.Client
}

// NewGradle 构造 Gradle 源适配器。
func NewGradle(client *http.Client) *gradle {
	return &gradle{base: base{base: gradleDefaultBase}, client: client}
}

func (s *gradle) ID() string   { return "gradle" }
func (s *gradle) Name() string { return "Gradle" }

type gradleVersion struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
	ChecksumURL string `json:"checksumUrl"`
	Milestone   bool   `json:"milestone"`
	RC1         bool   `json:"rc1"`
	RC2         bool   `json:"rc2"`
	Snapshot    bool   `json:"snapshot"`
	Nightly     bool   `json:"nightly"`
	ActiveRC    bool   `json:"activeRc"`
}

func (s *gradle) Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error) {
	_ = p // Gradle 发行包为跨平台 zip
	var versions []gradleVersion
	url := s.join("/versions/all")
	if err := getJSON(ctx, s.client, url, &versions); err != nil {
		return nil, fmt.Errorf("获取 Gradle 版本清单失败: %w", err)
	}
	var rels []*domain.Release
	for _, v := range versions {
		if v.Milestone || v.RC1 || v.RC2 || v.Snapshot || v.Nightly || v.ActiveRC {
			continue
		}
		rels = append(rels, &domain.Release{
			Version: v.Version,
			Stable:  true,
			Artifact: &domain.Artifact{
				URL:          v.DownloadURL,
				ChecksumURL:  v.ChecksumURL,
				ChecksumType: "sha256",
				Ext:          "zip",
				Strip:        1,
			},
		})
	}
	return rels, nil
}
