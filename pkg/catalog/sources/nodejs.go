package sources

import (
	"context"
	"fmt"
	"net/http"

	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

const nodejsDefaultBase = "https://nodejs.org"

// nodejsMirrorBase 国内可达的镜像（格式与官方 index.json 一致）。
const nodejsMirrorBase = "https://registry.npmmirror.com/-/binary/node"

type nodejs struct {
	base
	client *http.Client
}

// NewNodeJS 构造 Node.js 官方源适配器。
func NewNodeJS(client *http.Client) *nodejs {
	return &nodejs{base: base{base: nodejsDefaultBase}, client: client}
}

func (s *nodejs) ID() string   { return "nodejs" }
func (s *nodejs) Name() string { return "Node.js Official" }

type nodeVersion struct {
	Version string      `json:"version"` // 如 v22.11.0
	LTS     interface{} `json:"lts"`     // false 或代号字符串
	Files   []string    `json:"files"`
}

// nodePlatformKey 返回 Node 官方目录中的平台 key。
func nodePlatformKey(p platform.Platform) string {
	switch p.OS {
	case "linux":
		if p.Arch == "arm64" {
			return "linux-arm64"
		}
		return "linux-x64"
	case "darwin":
		if p.Arch == "arm64" {
			return "osx-arm64"
		}
		return "osx-x64"
	case "windows":
		if p.Arch == "arm64" {
			return "win-arm64"
		}
		return "win-x64"
	}
	return ""
}

// fetchIndex 从官方源拉取版本清单，失败时回退到国内镜像。
func (s *nodejs) fetchIndex(ctx context.Context) ([]nodeVersion, error) {
	var versions []nodeVersion
	url := s.join("/dist/index.json")
	if err := getJSON(ctx, s.client, url, &versions); err == nil {
		return versions, nil
	}
	// 回退镜像（与官方 index.json 字段一致）。
	mirrorURL := nodejsMirrorBase + "/index.json"
	var mirror []nodeVersion
	if err := getJSON(ctx, s.client, mirrorURL, &mirror); err != nil {
		return nil, fmt.Errorf("获取 Node.js 版本清单失败（官方源与镜像均不可达）: %w", err)
	}
	return mirror, nil
}

func (s *nodejs) Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error) {
	key := nodePlatformKey(p)
	if key == "" {
		return nil, fmt.Errorf("Node.js 暂不支持平台 %s", p.String())
	}
	versions, err := s.fetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	ext := "tar.gz"
	if p.IsWindows() {
		ext = "zip"
	}
	var rels []*domain.Release
	for _, v := range versions {
		hasFile := false
		for _, f := range v.Files {
			if f == key {
				hasFile = true
				break
			}
		}
		if !hasFile {
			continue
		}
		lts := false
		switch t := v.LTS.(type) {
		case string:
			lts = t != ""
		case bool:
			lts = t
		}
		baseURL := s.join("/dist/" + v.Version)
		rels = append(rels, &domain.Release{
			Version: v.Version, // 如 v22.11.0
			Stable:  true,
			LTS:     lts,
			Artifact: &domain.Artifact{
				URL:          baseURL + "/node-" + v.Version + "-" + key + "." + ext,
				ChecksumURL:  baseURL + "/SHASUMS256.txt",
				ChecksumType: "sha256",
				Ext:          ext,
				Strip:        1,
			},
		})
	}
	return rels, nil
}
