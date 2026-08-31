package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

func testPlatform() platform.Platform { return platform.FromParts("linux", "amd64") }

func TestGolangFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"version":"go1.23.4","files":[
				{"filename":"go1.23.4.linux-amd64.tar.gz","os":"linux","arch":"amd64","sha256":"abc123","kind":"archive"},
				{"filename":"go1.23.4.src.tar.gz","os":"linux","arch":"amd64","sha256":"x","kind":"source"}
			]},
			{"version":"go1.23.4rc1","files":[
				{"filename":"go1.23.4rc1.linux-amd64.tar.gz","os":"linux","arch":"amd64","sha256":"pre","kind":"archive"}
			]}
		]`))
	}))
	defer srv.Close()

	s := NewGolang(&http.Client{})
	s.SetBaseURL(srv.URL)
	rels, err := s.Fetch(context.Background(), testPlatform())
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 {
		t.Fatalf("应只返回稳定版本，得到 %d 个", len(rels))
	}
	if rels[0].Version != "go1.23.4" || rels[0].Artifact.SHA256 != "abc123" {
		t.Errorf("解析错误: %+v", rels[0])
	}
}

func TestNodeJSFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"version":"v22.11.0","lts":"Jod","files":["linux-x64","linux-arm64","osx-x64","win-x64"]},
			{"version":"v23.0.0","lts":false,"files":["linux-x64"]}
		]`))
	}))
	defer srv.Close()

	s := NewNodeJS(&http.Client{})
	s.SetBaseURL(srv.URL)
	rels, err := s.Fetch(context.Background(), testPlatform())
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 {
		t.Fatalf("应返回 2 个版本，得到 %d", len(rels))
	}
	var ltsFound bool
	checksumURL := ""
	for _, r := range rels {
		if r.Version == "v22.11.0" {
			ltsFound = r.LTS
			checksumURL = r.Artifact.ChecksumURL
		}
	}
	if !ltsFound {
		t.Error("v22.11.0 应标记为 LTS")
	}
	if checksumURL == "" || !strings.Contains(checksumURL, "/SHASUMS256.txt") {
		t.Errorf("校验值地址异常: %s", checksumURL)
	}
}

func TestAdoptiumFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/info/available_releases":
			w.Write([]byte(`{"available_releases":[17,21]}`))
		case "/v3/assets/latest/21/hotspot":
			w.Write([]byte(`[{"version":{"openjdk_version":"21.0.5+11-TS"},"binary":{"package":{
				"name":"OpenJDK21U-jdk_x64_linux_hotspot_21.0.5_11.tar.gz",
				"link":"https://example.com/jdk.tar.gz","checksum":"deadbeef"}}}]`))
		case "/v3/assets/latest/17/hotspot":
			w.Write([]byte(`[{"version":{"openjdk_version":"17.0.10+7-GA"},"binary":{"package":{
				"name":"OpenJDK17U-jdk_x64_linux_hotspot_17.0.10_7.zip",
				"link":"https://example.com/jdk17.zip","checksum":"cafe"}}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	s := NewAdoptium(&http.Client{})
	s.SetBaseURL(srv.URL)
	rels, err := s.Fetch(context.Background(), testPlatform())
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 {
		t.Fatalf("应返回 2 个版本，得到 %d", len(rels))
	}
	byVer := map[string]*domain.Release{}
	for _, r := range rels {
		byVer[r.Version] = r
	}
	if v := byVer["21.0.5+11-tem"]; v == nil || v.Artifact.SHA256 != "deadbeef" || v.Artifact.URL != "https://example.com/jdk.tar.gz" {
		t.Errorf("21 版本解析错误: %+v", v)
	}
	if v := byVer["17.0.10+7-tem"]; v == nil || v.Artifact.ArchiveExt() != "zip" {
		t.Errorf("17 版本应为 zip: %+v", v)
	}
}

func TestMavenFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
			<a href="3.9.9/">3.9.9/</a>
			<a href="3.8.8/">3.8.8/</a>
			<a href="3.9.0-alpha1/">3.9.0-alpha1/</a>
		</body></html>`))
	}))
	defer srv.Close()

	s := NewMaven(&http.Client{})
	s.SetBaseURL(srv.URL)
	rels, err := s.Fetch(context.Background(), testPlatform())
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 {
		t.Fatalf("应返回 2 个稳定版本（排除 alpha），得到 %d", len(rels))
	}
	art := rels[0].Artifact
	// 镜像模式（base 注入）：主地址跟随镜像，归档作为回退。
	if !strings.HasPrefix(art.URL, srv.URL) {
		t.Errorf("主地址应走镜像: %s", art.URL)
	}
	if len(art.FallbackURLs) == 0 || !strings.HasPrefix(art.FallbackURLs[0], "https://archive.apache.org/dist/") {
		t.Errorf("应有 archive.apache.org 回退地址: %v", art.FallbackURLs)
	}
	if art.ChecksumType != "sha512" {
		t.Errorf("checksum 类型应为 sha512: %s", art.ChecksumType)
	}
	// 校验值文件应与下载地址同源，并有对应回退。
	if art.ChecksumURL != art.URL+".sha512" {
		t.Errorf("ChecksumURL 应为主地址 + .sha512: %s", art.ChecksumURL)
	}
	if len(art.FallbackChecksumURLs) != 1 || art.FallbackChecksumURLs[0] != art.FallbackURLs[0]+".sha512" {
		t.Errorf("校验值回退地址错误: %v", art.FallbackChecksumURLs)
	}
}

func TestGradleFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"version":"8.10.2","downloadUrl":"https://services.gradle.org/distributions/gradle-8.10.2-bin.zip","checksumUrl":"https://services.gradle.org/distributions/gradle-8.10.2-bin.zip.sha256"},
			{"version":"8.10.2-rc-1","downloadUrl":"https://services.gradle.org/distributions/gradle-8.10.2-rc-1-bin.zip","checksumUrl":"x","rc1":true},
			{"version":"9.0.0-milestone-1","downloadUrl":"https://services.gradle.org/distributions/gradle-9.0.0-milestone-1-bin.zip","checksumUrl":"y","milestone":true}
		]`))
	}))
	defer srv.Close()

	s := NewGradle(&http.Client{})
	s.SetBaseURL(srv.URL)
	rels, err := s.Fetch(context.Background(), testPlatform())
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 || rels[0].Version != "8.10.2" {
		t.Fatalf("应只返回稳定版本: %+v", rels)
	}
	if rels[0].Artifact.URL == "" || rels[0].Artifact.ChecksumURL == "" {
		t.Error("缺少下载/校验地址")
	}
}
