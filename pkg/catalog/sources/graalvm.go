package sources

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

const graalvmDefaultBase = "https://api.github.com"

type graalvm struct {
	base
	client *http.Client
}

// NewGraalVM 构造 GraalVM Community 源适配器。
func NewGraalVM(client *http.Client) *graalvm {
	return &graalvm{base: base{base: graalvmDefaultBase}, client: client}
}

func (s *graalvm) ID() string   { return "graalvm" }
func (s *graalvm) Name() string { return "GraalVM Community" }

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// parseGraalAsset 解析 GraalVM 资产名，返回 (版本段, 是否匹配当前平台)。
// 名称形如：graalvm-community-jdk-25i3-25.0.4.1_linux-aarch64_bin.tar.gz
//   版本段可能带迭代号前缀（如 25i3-25.0.4.1），此处取其中的点分版本号 25.0.4.1。
func parseGraalAsset(name string, p platform.Platform) (bool, string) {
	base := strings.TrimSuffix(name, ".tar.gz")
	if strings.HasSuffix(name, ".zip") {
		base = strings.TrimSuffix(name, ".zip")
	}
	if !strings.HasPrefix(base, "graalvm-community-jdk-") {
		return false, ""
	}
	rest := strings.TrimPrefix(base, "graalvm-community-jdk-")
	parts := strings.Split(rest, "_")
	if len(parts) < 3 || parts[len(parts)-1] != "bin" {
		return false, ""
	}
	// 版本段：去掉可能的迭代前缀（如 25i3-25.0.4.1 -> 25.0.4.1）。
	verPart := parts[0]
	if idx := strings.Index(verPart, "-"); idx >= 0 {
		verPart = verPart[idx+1:]
	}
	// 目标平台匹配：GraalVM 命名 linux-x64 / linux-aarch64 / macos-x64 / macos-aarch64 / windows-x64。
	osMap := map[string]string{"linux": "linux", "macos": "darwin", "windows": "windows"}
	archMap := map[string]string{"x64": "amd64", "aarch64": "arm64"}
	osPart := parts[1]
	osParts := strings.Split(osPart, "-")
	if len(osParts) != 2 {
		return false, ""
	}
	if osMap[osParts[0]] != p.OS || archMap[osParts[1]] != p.Arch {
		return false, ""
	}
	return true, verPart
}

func (s *graalvm) Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error) {
	var releases []ghRelease
	url := s.join("/repos/graalvm/graalvm-ce-builds/releases?per_page=100")
	if err := getJSON(ctx, s.client, url, &releases); err != nil {
		return nil, fmt.Errorf("获取 GraalVM 版本清单失败: %w", err)
	}
	var rels []*domain.Release
	seen := map[string]bool{}
	for _, r := range releases {
		for _, a := range r.Assets {
			ok, ver := parseGraalAsset(a.Name, p)
			if !ok || ver == "" {
				continue
			}
			id := ver + "-graal"
			if seen[id] {
				continue
			}
			seen[id] = true
			ext := "tar.gz"
			if strings.HasSuffix(a.Name, ".zip") {
				ext = "zip"
			}
			rels = append(rels, &domain.Release{
				Version: id,
				Stable:  true,
				Artifact: &domain.Artifact{
					URL:          a.URL,
					Ext:          ext,
					Strip:        1,
					ChecksumType: "sha256", // GraalVM 无校验文件，下载后仅提示跳过
				},
			})
			break
		}
	}
	return rels, nil
}
