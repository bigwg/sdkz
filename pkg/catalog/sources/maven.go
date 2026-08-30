package sources

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

const (
	mavenDefaultBase = "https://dlcdn.apache.org"
	mavenArchiveBase = "https://archive.apache.org/dist"
	// mavenListBase 版本清单地址。dlcdn 是 CDN，仅保留当前在架版本，
	// 旧版本发新后即被移除；全量历史版本需从归档服务器获取。
	mavenListBase = mavenArchiveBase + "/maven/maven-3/"
)

type maven struct {
	base
	client *http.Client
}

// NewMaven 构造 Apache Maven 源适配器。
func NewMaven(client *http.Client) *maven {
	return &maven{client: client}
}

func (s *maven) ID() string   { return "maven" }
func (s *maven) Name() string { return "Apache Maven" }

var mavenDirRe = regexp.MustCompile(`href="([0-9][0-9.]*)/"`)

// listURL 版本清单地址：未注入自定义 base（镜像/测试假源）时走 Apache 全量归档，
// 否则跟随 base（镜像内路径结构一致）。
func (s *maven) listURL() string {
	if s.base.base != "" {
		return s.join("/maven/maven-3/")
	}
	return mavenListBase
}

func (s *maven) Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error) {
	ext := "tar.gz"
	if p.IsWindows() {
		ext = "zip"
	}
	html, err := getText(ctx, s.client, s.listURL(), 1<<20)
	if err != nil {
		return nil, fmt.Errorf("获取 Maven 版本清单失败: %w", err)
	}
	seen := map[string]bool{}
	var rels []*domain.Release
	for _, m := range mavenDirRe.FindAllStringSubmatch(html, -1) {
		v := m[1]
		if seen[v] {
			continue
		}
		seen[v] = true
		if domain.IsPreReleaseString(v) {
			continue
		}
		main := fmt.Sprintf("%s/maven/maven-3/%s/binaries/apache-maven-%s-bin.%s", s.join(mavenDefaultBase), v, v, ext)
		fallback := fmt.Sprintf("%s/maven/maven-3/%s/binaries/apache-maven-%s-bin.%s", mavenArchiveBase, v, v, ext)
		rels = append(rels, &domain.Release{
			Version: v,
			Stable:  true,
			Artifact: &domain.Artifact{
				URL:          main,
				FallbackURLs: []string{fallback},
				ChecksumURL:  main + ".sha512",
				ChecksumType: "sha512",
				Ext:          ext,
				Strip:        1,
			},
		})
	}
	return rels, nil
}
