// Package linker 实现 current 指针的跨平台自动降级：
// symlink → Windows junction（mklink /J）→ 目录复制。
package linker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Mode 表示指针实现模式。
type Mode int

const (
	Symlink Mode = iota
	Junction
	Copy
)

func (m Mode) String() string {
	switch m {
	case Symlink:
		return "symlink"
	case Junction:
		return "junction"
	case Copy:
		return "copy"
	}
	return "unknown"
}

// Manager 管理某一候选的 current 指针。
type Manager struct {
	root string // SDKZ 根目录
}

// NewManager 构造指针管理器。
func NewManager(root string) *Manager { return &Manager{root: root} }

// meta 记录指针模式，避免每次切换重复探测。
type meta struct {
	Mode Mode `json:"mode"`
}

func (m *Manager) metaPath(candID string) string {
	return filepath.Join(m.root, "candidates", candID, ".sdkz-meta.json")
}

func (m *Manager) linkPath(candID string) string {
	return filepath.Join(m.root, "candidates", candID, "current")
}

func (m *Manager) readMode(candID string) (Mode, bool) {
	data, err := os.ReadFile(m.metaPath(candID))
	if err != nil {
		return Symlink, false
	}
	var mm meta
	if json.Unmarshal(data, &mm) != nil {
		return Symlink, false
	}
	return mm.Mode, true
}

func (m *Manager) writeMode(candID string, mode Mode) error {
	mm := meta{Mode: mode}
	data, err := json.Marshal(mm)
	if err != nil {
		return err
	}
	return os.WriteFile(m.metaPath(candID), data, 0o644)
}

// Set 将 current 指向 src 版本目录，返回使用的模式。
func (m *Manager) Set(candID, src string) (Mode, error) {
	link := m.linkPath(candID)
	if mode, ok := m.readMode(candID); ok {
		// 已有模式：按该模式更新。
		if err := m.removeLink(link, mode); err != nil {
			return mode, err
		}
		if err := m.createLink(link, src, mode); err != nil {
			return mode, err
		}
		return mode, nil
	}
	// 首次：自动探测降级链。
	for _, mode := range []Mode{Symlink, Junction, Copy} {
		if runtime.GOOS != "windows" && mode == Junction {
			continue
		}
		if err := m.removeLink(link, Copy); err != nil {
			continue
		}
		if err := m.createLink(link, src, mode); err != nil {
			continue
		}
		_ = m.writeMode(candID, mode)
		return mode, nil
	}
	return Copy, fmt.Errorf("无法创建 current 指针（symlink/junction/copy 均失败）")
}

// Unlink 移除 current 指针（不动版本目录）。
func (m *Manager) Unlink(candID string) error {
	mode, ok := m.readMode(candID)
	if !ok {
		mode = Symlink
	}
	return m.removeLink(m.linkPath(candID), mode)
}

// Resolve 返回 current 指向的版本目录绝对路径。
// 注：copy 模式下 current 本身即版本目录。
func (m *Manager) Resolve(candID string) (string, error) {
	link := m.linkPath(candID)
	mode, ok := m.readMode(candID)
	if !ok {
		mode = Symlink
	}
	switch mode {
	case Symlink, Junction:
		target, err := os.Readlink(link)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(link), target)
		}
		return filepath.Clean(target), nil
	default: // Copy
		return link, nil
	}
}

// Current 返回当前版本名（目录 base）。
func (m *Manager) Current(candID string) (string, error) {
	resolved, err := m.Resolve(candID)
	if err != nil {
		return "", err
	}
	return filepath.Base(resolved), nil
}

func (m *Manager) removeLink(link string, mode Mode) error {
	switch mode {
	case Symlink, Junction:
		// 仅删除链接本身；reparse point 安全处理由 Go 保证。
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return err
		}
	case Copy:
		if err := os.RemoveAll(link); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (m *Manager) createLink(link, src string, mode Mode) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	switch mode {
	case Symlink:
		if err := os.Symlink(srcAbs, link); err != nil {
			return err
		}
	case Junction:
		linkAbs, err := filepath.Abs(link)
		if err != nil {
			return err
		}
		if err := junction(linkAbs, srcAbs); err != nil {
			return err
		}
	case Copy:
		if err := copyDir(srcAbs, link, 0); err != nil {
			return err
		}
	}
	return nil
}

// junction 在 Windows 上创建目录联接（免管理员）。
func junction(link, src string) error {
	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J 失败: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// copyDir 递归复制目录内容。
func copyDir(src, dst string, depth int) error {
	if depth > 0 {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		sp := filepath.Join(src, e.Name())
		dp := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if e.Name() == "current" {
				continue // 避免递归复制指针
			}
			if err := os.MkdirAll(dp, 0o755); err != nil {
				return err
			}
			if err := copyDir(sp, dp, depth+1); err != nil {
				return err
			}
			continue
		}
		if e.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(sp)
			if err != nil {
				continue
			}
			if err := os.Symlink(target, dp); err != nil {
				// 复制链接失败不阻断
			}
			continue
		}
		if err := copyFile(sp, dp); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, fi.Mode().Perm())
}
