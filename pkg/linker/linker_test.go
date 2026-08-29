package linker

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSetAndResolve(t *testing.T) {
	root := t.TempDir()
	m := NewManager(root)

	// 准备两个版本目录。
	v1 := filepath.Join(root, "candidates", "java", "1.0.0")
	v2 := filepath.Join(root, "candidates", "java", "2.0.0")
	for _, d := range []string{v1, v2} {
		if err := os.MkdirAll(filepath.Join(d, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "bin", "java"), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	mode, err := m.Set("java", v1)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" && mode == Copy {
		// Windows 无 mklink 权限时允许 copy 模式。
		t.Logf("Windows 上使用 %v 模式", mode)
	}
	cur, err := m.Current("java")
	if err != nil {
		t.Fatal(err)
	}
	if cur != "1.0.0" {
		t.Errorf("Current = %q，期望 1.0.0", cur)
	}

	// 切换到 v2。
	if _, err := m.Set("java", v2); err != nil {
		t.Fatal(err)
	}
	cur, err = m.Current("java")
	if err != nil {
		t.Fatal(err)
	}
	if cur != "2.0.0" {
		t.Errorf("切换后 Current = %q，期望 2.0.0", cur)
	}

	// Unlink 后 Current 应报错。
	if err := m.Unlink("java"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Current("java"); err == nil {
		t.Error("Unlink 后 Current 应失败")
	}
}

func TestResolveCopyMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix 环境验证 copy 模式")
	}
	root := t.TempDir()
	m := NewManager(root)
	v1 := filepath.Join(root, "candidates", "go", "go1.23.4")
	if err := os.MkdirAll(filepath.Join(v1, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v1, "bin", "go"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Set("go", v1); err != nil {
		t.Fatal(err)
	}
	// symlink 可用，应返回 symlink 模式；此时 Resolve 指向 v1。
	resolved, err := m.Resolve("go")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(resolved) != "go1.23.4" {
		t.Errorf("Resolve = %q", resolved)
	}
}
