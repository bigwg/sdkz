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

// nodePlatformSpec 描述目标平台在 Node.js 官方 index.json 中的 files 键，
// 以及下载产物文件名中的平台标识。
//
// 两者并不一致，这是 Windows / macOS 曾"解析不到任何版本"的根因：
//
//	Windows：files=win-x64-zip / win-arm64-zip   产物=node-vX-win-x64.zip
//	macOS  ：files=osx-x64-tar / osx-arm64-tar   产物=node-vX-darwin-x64.tar.gz
//	Linux  ：files=linux-x64 / linux-arm64        产物=node-vX-linux-x64.tar.gz
//
// fileKeys 中保留裸名（如 win-x64）作为旧版清单/镜像格式的兼容项。
type nodePlatformSpec struct {
	fileKeys []string // 任一命中即表示该版本提供了当前平台可安装的产物
	name     string   // 产物文件名中的平台标识
	ext      string   // 归档扩展名
}

func nodePlatformSpecFor(p platform.Platform) (nodePlatformSpec, bool) {
	switch p.OS {
	case "linux":
		if p.Arch == "arm64" {
			return nodePlatformSpec{[]string{"linux-arm64"}, "linux-arm64", "tar.gz"}, true
		}
		return nodePlatformSpec{[]string{"linux-x64"}, "linux-x64", "tar.gz"}, true
	case "darwin":
		if p.Arch == "arm64" {
			return nodePlatformSpec{[]string{"osx-arm64-tar", "osx-arm64", "darwin-arm64"}, "darwin-arm64", "tar.gz"}, true
		}
		return nodePlatformSpec{[]string{"osx-x64-tar", "osx-x64", "darwin-x64"}, "darwin-x64", "tar.gz"}, true
	case "windows":
		if p.Arch == "arm64" {
			return nodePlatformSpec{[]string{"win-arm64-zip", "win-arm64"}, "win-arm64", "zip"}, true
		}
		return nodePlatformSpec{[]string{"win-x64-zip", "win-x64"}, "win-x64", "zip"}, true
	}
	return nodePlatformSpec{}, false
}

// hasArtifact 判断该版本是否提供了当前平台可安装的归档产物。
func (s nodePlatformSpec) hasArtifact(files []string) bool {
	for _, f := range files {
		for _, k := range s.fileKeys {
			if f == k {
				return true
			}
		}
	}
	return false
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
	spec, ok := nodePlatformSpecFor(p)
	if !ok {
		return nil, fmt.Errorf("Node.js 暂不支持平台 %s", p.String())
	}
	versions, err := s.fetchIndex(ctx)
	if err != nil {
		return nil, err
	}
	var rels []*domain.Release
	for _, v := range versions {
		if !spec.hasArtifact(v.Files) {
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
				URL:          baseURL + "/node-" + v.Version + "-" + spec.name + "." + spec.ext,
				ChecksumURL:  baseURL + "/SHASUMS256.txt",
				ChecksumType: "sha256",
				Ext:          spec.ext,
				Strip:        1,
			},
		})
	}
	return rels, nil
}
