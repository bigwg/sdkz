package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTarGz 生成带顶层目录的 tar.gz（模拟 JDK 包）。
func makeTarGz(t *testing.T, top string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: top + "/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name: top + "/" + name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func sha256hex(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

func TestIntegrationInstallUseDefaultUninstall(t *testing.T) {
	// 简化：直接构造 server 并拿到 URL。
	jdk21 := makeTarGz(t, "jdk-21.0.5", map[string]string{
		"bin/java": "#!/bin/sh\necho java21\n",
		"release":  "JAVA_VERSION=21",
	})
	sha21 := sha256hex(jdk21)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/info/available_releases":
			fmt.Fprint(w, `{"available_releases":[21]}`)
		case "/v3/assets/latest/21/hotspot":
			fmt.Fprintf(w, `[{"version":{"openjdk_version":"21.0.5+11-TS"},"binary":{"package":{
				"name":"jdk21.tar.gz","link":"%s/jdk21.tar.gz","checksum":"%s"}}}]`, srv.URL, sha21)
		case "/jdk21.tar.gz":
			w.Header().Set("Content-Type", "application/gzip")
			w.Write(jdk21)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	m, err := NewWithRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	m.SetSourceBaseURL("temurin", srv.URL)

	ctx := context.Background()

	// 1. 安装 java 21（默认 Temurin）。
	rel, err := m.Install(ctx, "java", "21", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "21.0.5+11-tem" {
		t.Fatalf("安装版本 = %q", rel.Version)
	}
	verDir := m.InstalledDir("java", rel.Version)
	if _, err := os.Stat(filepath.Join(verDir, "bin", "java")); err != nil {
		t.Fatalf("安装产物缺失: %v", err)
	}

	// 2. 设置全局默认。
	block, err := m.SetDefault("java", rel.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(block, "JAVA_HOME") {
		t.Errorf("default 输出应含 JAVA_HOME: %s", block)
	}
	cur, err := m.Current("java")
	if err != nil {
		t.Fatal(err)
	}
	if cur != rel.Version {
		t.Errorf("Current = %q", cur)
	}

	// 3. use（当前 shell 生效）。
	ublock, err := m.Use("java", rel.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ublock, "JAVA_HOME") || !strings.Contains(ublock, "21.0.5+11-tem") {
		t.Errorf("use 输出异常: %s", ublock)
	}

	// 4. 已安装列表。
	installed, err := m.ListInstalled("java")
	if err != nil || len(installed) != 1 {
		t.Fatalf("ListInstalled = %v, err=%v", installed, err)
	}

	// 5. 卸载当前版本应被拒绝。
	if err := m.Uninstall("java", rel.Version); err == nil {
		t.Error("卸载当前版本应报错")
	}

	// 6. 取消指针后可卸载。
	if err := unlinkHelper(m, "java"); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall("java", rel.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(verDir); err == nil {
		t.Error("卸载后目录仍存在")
	}
}

// unlinkHelper 通过直接操作 linker 解除 current 指针（service 未暴露 Unlink）。
func unlinkHelper(m *Manager, candID string) error {
	// 使用 service 未暴露的能力：这里通过再次 default 到不存在版本无法实现，
	// 直接删除 meta 与 current。
	dir := filepath.Join(m.Root(), "candidates", candID)
	if err := os.RemoveAll(filepath.Join(dir, "current")); err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, ".sdkz-meta.json"))
}
