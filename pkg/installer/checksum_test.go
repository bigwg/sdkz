package installer

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdkz/pkg/download"
)

// 取自 Node.js v24.22.0 官方 SHASUMS256.txt 的真实片段（按平台列出全部哈希）。
const shasumsMulti = `cbb1d625fbfa61b23fd4a949ea41c9b08bb7905fbc75403bb11d4bcca73a5566  node-v24.22.0-aix-ppc64.tar.gz
3161e384f52d95e80bbc85c7ec6fe675528790647b7a3ec71e6ea6d53ce66b29  node-v24.22.0-arm64.msi
6a9724d9b50a5f22b4816cded441b5a8d9ddae546191a5e5b7efa5a5e2ee26d5  node-v24.22.0-darwin-arm64.tar.gz
28ea0a438de9a13bd7891fd758eee6c7819ee8af5442dfd8d45bd65e56d4228c  node-v24.22.0-linux-x64.tar.gz
f0e4c2f76c58916ec258f246851bea091d14d4247a2fc3e18694461b1816e13b  node-v24.22.0-linux-arm64.tar.gz
`

func TestParseChecksum_MultiEntryMatch(t *testing.T) {
	// 回归：多平台清单必须按文件名取哈希，而不是第一行（aix-ppc64）。
	got, err := parseChecksum(shasumsMulti, "sha256", "node-v24.22.0-linux-x64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	want := "28ea0a438de9a13bd7891fd758eee6c7819ee8af5442dfd8d45bd65e56d4228c"
	if got != want {
		t.Errorf("哈希不匹配:\n got %s\nwant %s", got, want)
	}
}

func TestParseChecksum_MultiEntryCaseInsensitive(t *testing.T) {
	got, err := parseChecksum(shasumsMulti, "sha256", "Node-V24.22.0-Linux-X64.TAR.GZ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "28ea0a438de9a13bd7891fd758eee6c7819ee8af5442dfd8d45bd65e56d4228c" {
		t.Errorf("大小写不敏感匹配失败: %s", got)
	}
}

func TestParseChecksum_MissingEntry(t *testing.T) {
	// 清单中不存在目标文件时，必须报错而不是错取其他平台的哈希。
	_, err := parseChecksum(shasumsMulti, "sha256", "node-v24.22.0-linux-armv7l.tar.gz")
	if !errors.Is(err, ErrChecksumNotFound) {
		t.Errorf("应返回 ErrChecksumNotFound，实际: %v", err)
	}
}

func TestParseChecksum_NoFilename(t *testing.T) {
	// 未提供文件名且清单含多文件哈希时，无法确定目标，报错。
	_, err := parseChecksum(shasumsMulti, "sha256", "")
	if !errors.Is(err, ErrChecksumNotFound) {
		t.Errorf("应返回 ErrChecksumNotFound，实际: %v", err)
	}
}

func TestParseChecksum_BareHash(t *testing.T) {
	// 单值裸哈希文件（如 Maven/Gradle 的 .sha256/.sha512）。
	got, err := parseChecksum("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n", "sha256", "gradle-8.10.2-bin.zip")
	if err != nil {
		t.Fatal(err)
	}
	if got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("裸哈希解析失败: %s", got)
	}
}

func TestParseChecksum_BSDStyle(t *testing.T) {
	// BSD 风格 "*哈希  文件名"。
	text := "*28ea0a438de9a13bd7891fd758eee6c7819ee8af5442dfd8d45bd65e56d4228c  node-v24.22.0-linux-x64.tar.gz\n"
	got, err := parseChecksum(text, "sha256", "node-v24.22.0-linux-x64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != "28ea0a438de9a13bd7891fd758eee6c7819ee8af5442dfd8d45bd65e56d4228c" {
		t.Errorf("BSD 风格解析失败: %s", got)
	}
}

func TestParseChecksum_SHA512(t *testing.T) {
	h512 := "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"
	got, err := parseChecksum(h512+"\n", "sha512", "any-file.zip")
	if err != nil {
		t.Fatal(err)
	}
	if got != h512 {
		t.Errorf("sha512 裸哈希解析失败: %s", got)
	}
	// sha256 长度按 sha512 算法解析应失败。
	if _, err := parseChecksum(h512, "sha256", "any-file.zip"); err == nil {
		t.Error("sha512 哈希不应按 sha256 解析成功")
	}
}

func TestFetchChecksum_MultiEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(shasumsMulti))
	}))
	defer srv.Close()

	got, err := FetchChecksum(context.Background(), srv.Client(), srv.URL+"/SHASUMS256.txt", "sha256",
		"node-v24.22.0-linux-x64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != "28ea0a438de9a13bd7891fd758eee6c7819ee8af5442dfd8d45bd65e56d4228c" {
		t.Errorf("端到端取哈希失败: %s", got)
	}
}

func TestFetchChecksum_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := FetchChecksum(context.Background(), srv.Client(), srv.URL+"/missing.txt", "sha256", "x.tar.gz")
	var nf *download.NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("404 应返回 *download.NotFoundError，实际: %v", err)
	}
}
