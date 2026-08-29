// Package config 管理 sdkz 配置（config.toml 与环境变量覆盖）。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config 是 sdkz 的运行时配置。
type Config struct {
	Root           string            `toml:"root"`             // SDKZ 数据目录
	Offline        bool              `toml:"offline"`          // 离线模式
	Mirror         map[string]string `toml:"mirror"`           // host → base URL 镜像规则
	AutoConfirm    bool              `toml:"auto_confirm"`     // 免交互确认
	SelfUpdateRepo string            `toml:"self_update_repo"` // owner/repo，用于 selfupdate
	Concurrency    int               `toml:"concurrency"`      // 并发请求数
}

const (
	envRoot = "SDKZ_DIR"
)

// DefaultRoot 计算默认数据目录：优先 SDKZ_DIR，其次 ~/.sdkz。
func DefaultRoot() string {
	if r := os.Getenv(envRoot); r != "" {
		return r
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".sdkz"
	}
	return filepath.Join(home, ".sdkz")
}

// ConfigPath 返回配置文件的路径。
func ConfigPath(root string) string { return filepath.Join(root, "config.toml") }

// Load 加载配置。Root 解析顺序：SDKZ_DIR 环境变量 > 配置文件 root 字段 > ~/.sdkz。
func Load() (*Config, error) {
	root := DefaultRoot()
	cfg := &Config{Root: root, Concurrency: 4}

	// 若 root 下已存在配置文件，读取之（可能覆盖 root 字段）。
	if p := ConfigPath(root); fileExists(p) {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("读取配置失败: %w", err)
		}
		if len(data) > 0 {
			if err := toml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("解析配置 %s 失败: %w", p, err)
			}
		}
	}
	if cfg.Root == "" {
		cfg.Root = root
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	return cfg, nil
}

// Save 将配置写入 Root/config.toml。
func (c *Config) Save() error {
	if err := os.MkdirAll(c.Root, 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigPath(c.Root), data, 0o644)
}

// ApplyMirror 对下载 URL 应用镜像替换。
// 规则：key 为 "host[/path...]"（最长匹配优先），URL 以 "https://<key>/" 开头时
// 将此前缀整体替换为镜像 base（末尾自动补 "/"），其余路径保留。
func (c *Config) ApplyMirror(raw string) string {
	if raw == "" || len(c.Mirror) == 0 {
		return raw
	}
	// 确定 key 以最长匹配优先，避免部分前缀误配。
	keys := make([]string, 0, len(c.Mirror))
	for k := range c.Mirror {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, k := range keys {
		base := strings.TrimRight(c.Mirror[k], "/")
		if base == "" {
			continue
		}
		prefix := "https://" + k + "/"
		if strings.HasPrefix(raw, prefix) {
			rest := strings.TrimPrefix(raw, prefix)
			// 避免镜像 base 末尾与剩余路径首段重复（如 /dl + dl/xxx）。
			if last := base[strings.LastIndex(base, "/")+1:]; last != "" && strings.HasPrefix(rest, last+"/") {
				rest = strings.TrimPrefix(rest, last+"/")
			}
			return base + "/" + rest
		}
	}
	return raw
}

// ApplyMirrorToArtifact 对产物的所有 URL 应用镜像规则。
func (c *Config) ApplyMirrorToArtifact(urls map[string]string) map[string]string {
	out := make(map[string]string, len(urls))
	for k, u := range urls {
		out[k] = c.ApplyMirror(u)
	}
	return out
}

// CNPreset 返回内置国内镜像预设（键为 host 或 host/路径前缀）。
func CNPreset() map[string]string {
	return map[string]string{
		"nodejs.org":          "https://cdn.npmmirror.com/binaries/node",
		"go.dev/dl":           "https://goproxy.cn/dl",
		"dl.google.com/go":    "https://goproxy.cn/dl",
		"services.gradle.org": "https://mirrors.tuna.tsinghua.edu.cn/gradle",
		"dlcdn.apache.org":    "https://mirrors.aliyun.com/apache",
		"archive.apache.org":  "https://mirrors.aliyun.com/apache",
		"api.adoptium.net":    "", // adoptium 官方 API 无国内镜像，留空表示不替换
		"api.github.com":      "https://ghproxy.net/https://api.github.com",
	}
}

// UseCN 应用国内镜像预设。
func (c *Config) UseCN() error {
	if c.Mirror == nil {
		c.Mirror = map[string]string{}
	}
	for k, v := range CNPreset() {
		if v == "" {
			continue
		}
		c.Mirror[k] = v
	}
	return c.Save()
}

// ClearMirror 清空全部镜像规则。
func (c *Config) ClearMirror() error {
	c.Mirror = map[string]string{}
	return c.Save()
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// EnsureRoot 创建数据目录结构。
func (c *Config) EnsureRoot() error {
	if c.Root == "" {
		return errors.New("sdkz 数据目录为空")
	}
	for _, d := range []string{
		filepath.Join(c.Root, "candidates"),
		filepath.Join(c.Root, "tmp"),
		filepath.Join(c.Root, "metadata"),
		filepath.Join(c.Root, "bin"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", d, err)
		}
	}
	return nil
}
