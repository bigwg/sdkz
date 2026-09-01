package config

import (
	"os"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func readFileInto(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return toml.Unmarshal(data, out)
}

func TestApplyMirror(t *testing.T) {
	cfg := &Config{
		Mirror: map[string]string{
			"nodejs.org":       "https://cdn.npmmirror.com/binaries/node",
			"go.dev/dl":        "https://goproxy.cn/dl",
			"dl.google.com/go": "https://goproxy.cn/dl",
		},
	}
	cases := []struct {
		in   string
		want string
	}{
		{"https://nodejs.org/dist/v22.11.0/node-v22.11.0-linux-x64.tar.gz",
			"https://cdn.npmmirror.com/binaries/node/dist/v22.11.0/node-v22.11.0-linux-x64.tar.gz"},
		{"https://go.dev/dl/go1.23.4.linux-amd64.tar.gz",
			"https://goproxy.cn/dl/go1.23.4.linux-amd64.tar.gz"},
		{"https://dl.google.com/go/go1.23.4.linux-amd64.tar.gz",
			"https://goproxy.cn/dl/go1.23.4.linux-amd64.tar.gz"},
		{"https://api.adoptium.net/v3/info/available_releases",
			"https://api.adoptium.net/v3/info/available_releases"}, // 无镜像不动
	}
	for _, c := range cases {
		got := cfg.ApplyMirror(c.in)
		if got != c.want {
			t.Errorf("ApplyMirror(%s)\n  got: %s\n want: %s", c.in, got, c.want)
		}
	}
}

func TestLoadWithEnvRoot(t *testing.T) {
	t.Setenv("SDKZ_DIR", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Root == "" {
		t.Error("Root 为空")
	}
	if cfg.Concurrency <= 0 {
		t.Error("Concurrency 未初始化")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	cfg := &Config{Root: root, Mirror: map[string]string{"a.com": "https://b.com"}, Concurrency: 2}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	// 直接解析写出的文件（不依赖全局 SDKZ_DIR）。
	var loaded Config
	if err := readFileInto(ConfigPath(root), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Mirror["a.com"] != "https://b.com" {
		t.Errorf("回读不一致: %+v", loaded)
	}
	if loaded.Root != root {
		t.Errorf("Root 回读 = %q，期望 %q", loaded.Root, root)
	}
}
