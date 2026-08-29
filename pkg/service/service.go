// Package service 是 sdkz 的核心用例编排层，CLI 与未来 GUI 的唯一入口。
// 设计约束：
//   - 所有长任务方法接受 context.Context 与 ProgressFunc 回调；
//   - 方法返回结构化结果而非直接打印；
//   - Warn 用于非致命告警（CLI 接 stderr，GUI 可收集展示）。
package service

import (
	"fmt"
	"net/http"
	"os"

	"sdkz/pkg/catalog"
	"sdkz/pkg/config"
	"sdkz/pkg/domain"
	"sdkz/pkg/env"
	"sdkz/pkg/installer"
	"sdkz/pkg/linker"
)

// ProgressFunc 长任务进度回调（stage: 下载/校验/解压…）。
type ProgressFunc = domain.ProgressFunc

// ChooseFunc 多发行版选择回调（CLI 提供交互菜单，GUI 提供界面选择；返回候选索引）。
type ChooseFunc func(candidates []*domain.Release) (int, error)

// Manager 聚合全部用例。
type Manager struct {
	cfg    *config.Config
	cat    *catalog.Manager
	ins    *installer.Installer
	link   *linker.Manager
	client *http.Client
	Shell  string
	// Warn 默认输出到 stderr。
	Warn func(format string, args ...any)
}

// New 基于默认配置构造 Manager。
func New() (*Manager, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return NewWithConfig(cfg)
}

// NewWithRoot 使用指定数据目录构造（测试 / 多实例场景）。
func NewWithRoot(root string) (*Manager, error) {
	cfg := &config.Config{Root: root, Concurrency: 4, Mirror: map[string]string{}}
	return NewWithConfig(cfg)
}

// NewWithConfig 使用指定配置构造。
func NewWithConfig(cfg *config.Config) (*Manager, error) {
	if err := cfg.EnsureRoot(); err != nil {
		return nil, err
	}
	m := &Manager{
		cfg:    cfg,
		client: &http.Client{},
		Shell:  env.DetectShellOr("bash"),
	}
	m.Warn = func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "警告: "+format+"\n", args...)
	}
	m.cat = catalog.NewManager(cfg)
	m.cat.Warn = m.Warn
	m.ins = installer.NewInstaller(cfg)
	m.ins.Warn = m.Warn
	m.link = linker.NewManager(cfg.Root)
	return m, nil
}

// Config 返回当前配置。
func (m *Manager) Config() *config.Config { return m.cfg }

// Candidates 返回全部候选。
func (m *Manager) Candidates() []*domain.Candidate { return catalog.BuiltinCandidates() }

// FindCandidate 按 id 查找候选。
func (m *Manager) FindCandidate(id string) (*domain.Candidate, error) {
	c, ok := catalog.FindCandidate(id)
	if !ok {
		return nil, fmt.Errorf("未知候选 %q（可用: java, go, node, maven, gradle）", id)
	}
	return c, nil
}

// Root 返回 SDKZ 数据目录。
func (m *Manager) Root() string { return m.cfg.Root }

// InstalledDir 返回版本目录路径。
func (m *Manager) InstalledDir(candID, ver string) string {
	return m.ins.VersionDir(candID, ver)
}

// SetSourceBaseURL 覆盖某版本源的基础 URL（测试注入 / 特殊镜像）。
func (m *Manager) SetSourceBaseURL(sourceID, base string) {
	m.cat.SetBaseURL(sourceID, base)
}
