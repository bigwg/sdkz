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

func (s *maven) Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error) {
	ext := "tar.gz"
	if p.IsWindows() {
		ext = "zip"
	}

	// 清单：默认走 Apache 全量归档；注入自定义 base（镜像/测试）时跟随 base。
	// 在架清单（dlcdn）拉取失败仅降级为"全部走归档"，不影响版本展示。
	var listHTML, shelfHTML string
	var err error
	if s.base.base != "" {
		listHTML, err = getText(ctx, s.client, s.join("/maven/maven-3/"), 1<<20)
	} else {
		listHTML, err = getText(ctx, s.client, mavenListBase, 1<<20)
		if err == nil {
			shelfHTML, _ = getText(ctx, s.client, mavenDefaultBase+"/maven/maven-3/", 1<<20)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("获取 Maven 版本清单失败: %w", err)
	}

	shelf := map[string]bool{}
	for _, m := range mavenDirRe.FindAllStringSubmatch(shelfHTML, -1) {
		shelf[m[1]] = true
	}

	seen := map[string]bool{}
	var rels []*domain.Release
	for _, m := range mavenDirRe.FindAllStringSubmatch(listHTML, -1) {
		v := m[1]
		if seen[v] || domain.IsPreReleaseString(v) {
			continue
		}
		seen[v] = true

		dlcdnURL := fmt.Sprintf("%s/maven/maven-3/%s/binaries/apache-maven-%s-bin.%s", s.join(mavenDefaultBase), v, v, ext)
		archiveURL := fmt.Sprintf("%s/maven/maven-3/%s/binaries/apache-maven-%s-bin.%s", mavenArchiveBase, v, v, ext)

		art := &domain.Artifact{
			ChecksumType: "sha512",
			Ext:          ext,
			Strip:        1,
		}
		// 在架版本（或镜像模式）：主地址走 dlcdn/镜像（CDN 快），归档兜底；
		// 已下架版本：仅存在于归档，主地址直接走归档，避免无谓 404。
		if shelf[v] || s.base.base != "" {
			art.URL = dlcdnURL
			art.FallbackURLs = []string{archiveURL}
		} else {
			art.URL = archiveURL
		}
		art.ChecksumURL = art.URL + ".sha512"
		if len(art.FallbackURLs) > 0 {
			art.FallbackChecksumURLs = []string{art.FallbackURLs[0] + ".sha512"}
		}

		rels = append(rels, &domain.Release{
			Version:  v,
			Stable:   true,
			Artifact: art,
		})
	}
	return rels, nil
}
