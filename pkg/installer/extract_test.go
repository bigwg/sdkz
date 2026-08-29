package installer

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	// 顶层目录（strip=1 时被剥掉）。
	if err := tw.WriteHeader(&tar.Header{Name: "jdk-21.0.5/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		hdr := &tar.Header{
			Name:     "jdk-21.0.5/" + name,
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
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

func TestExtractTarGzStrip(t *testing.T) {
	data := makeTarGz(t, map[string]string{
		"bin/java":    "#!/bin/sh\necho java\n",
		"bin/javac":   "javac",
		"lib/modules": "modules",
		"release":     "JAVA_VERSION=21",
	})
	archive := filepath.Join(t.TempDir(), "jdk.tar.gz")
	if err := os.WriteFile(archive, data, 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if err := Extract(context.Background(), archive, dest, 1); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"bin/java", "bin/javac", "lib/modules", "release"} {
		if _, err := os.Stat(filepath.Join(dest, f)); err != nil {
			t.Errorf("缺少 %s: %v", f, err)
		}
	}
	// 顶层目录不应出现。
	if _, err := os.Stat(filepath.Join(dest, "jdk-21.0.5")); err == nil {
		t.Error("顶层目录未被剥离")
	}
	// 可执行位保留。
	fi, err := os.Stat(filepath.Join(dest, "bin/java"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&0o111 == 0 {
		t.Error("bin/java 缺少可执行位")
	}
}

func TestExtractZipSlipProtection(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// 恶意路径穿越。
	w, _ := zw.Create("../../evil.txt")
	w.Write([]byte("evil"))
	zw.Close()

	archive := filepath.Join(t.TempDir(), "evil.zip")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := Extract(context.Background(), archive, dest, 0); err == nil {
		t.Error("应拒绝路径穿越")
	}
	// 穿越文件不应写入。
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "evil.txt")); err == nil {
		t.Error("穿越文件被写出")
	}
}

func TestStripName(t *testing.T) {
	cases := []struct {
		name  string
		strip int
		want  string
		skip  bool
	}{
		{"jdk-21/bin/java", 1, "bin/java", false},
		{"jdk-21/", 1, "", true},
		{"jdk-21", 1, "", true},
		{"a/b/c", 2, "c", false},
		{"./jdk-21/bin/java", 1, "bin/java", false},
	}
	for _, c := range cases {
		got, skip := stripName(c.name, c.strip)
		if skip != c.skip || (skip == false && got != c.want) {
			t.Errorf("stripName(%q, %d) = (%q, %v)，期望 (%q, %v)", c.name, c.strip, got, skip, c.want, c.skip)
		}
	}
}
