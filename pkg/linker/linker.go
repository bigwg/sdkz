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

// envVarName 返回候选对应的 *_HOME 类环境变量名（小写形式用于内部映射）。
// 注意：本文件只负责 Windows 用户级环境变量的设置，与 domain.Candidate 解耦，
// 由调用方传入 homeEnv（如 JAVA_HOME）与 binDir（版本目录下的 bin 子目录绝对路径）。

// UserEnv 描述要为某候选写入的用户级环境变量。
type UserEnv struct {
	HomeEnv string // 如 JAVA_HOME / GOROOT / MAVEN_HOME / GRADLE_HOME，可为空
	HomeDir string // 版本目录绝对路径，写入 *_HOME
	BinDir  string // 版本目录下的 bin 绝对路径，追加进 PATH（去重）
}

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

// SetUserEnvWindows 在 Windows 上将环境变量持久写入用户级（HKCU\Environment）。
// - HomeEnv/HomeDir：写入用户级 *_HOME（如 JAVA_HOME）。HomeEnv 为空则跳过。
// - BinDir：以去重方式追加进用户级 PATH（若已存在则不动 PATH）。
// 使用 setx 命令实现，对 PowerShell / CMD / Git Bash 均通用，无需管理员权限。
// 注意：setx 仅影响未来进程；当前进程不会因此变更（调用方如需当前进程也生效，
// 应另行在进程内赋值）。
func SetUserEnvWindows(e UserEnv) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("SetUserEnvWindows 仅支持 Windows")
	}
	if e.HomeEnv != "" && e.HomeDir != "" {
		if err := setxUser(e.HomeEnv, e.HomeDir); err != nil {
			return fmt.Errorf("设置 %s 失败: %w", e.HomeEnv, err)
		}
	}
	if e.BinDir != "" {
		if err := appendUserPathIfMissing(e.BinDir); err != nil {
			return fmt.Errorf("更新 PATH 失败: %w", err)
		}
	}
	return nil
}

// setxUser 调用 setx 设置用户级环境变量（无需管理员）。
func setxUser(key, value string) error {
	cmd := exec.Command("cmd", "/c", "setx", key, value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// appendUserPathIfMissing 将 dir 以去重方式追加进用户级 PATH。
// 通过读取当前用户 PATH（从注册表经由 setx 的回显不可靠，故用 PowerShell 读取），
// 拼接后写回，避免每次切换导致 PATH 无限膨胀。
func appendUserPathIfMissing(dir string) error {
	// 用 PowerShell 读取用户级 PATH 当前值。
	getCmd := exec.Command("powershell", "-NoProfile", "-Command",
		"[Environment]::GetEnvironmentVariable('PATH','User')")
	out, err := getCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("读取用户 PATH 失败: %v: %s", err, strings.TrimSpace(string(out)))
	}
	cur := strings.TrimRight(strings.TrimSpace(string(out)), "\r\n")
	if pathContains(cur, dir) {
		return nil // 已存在，无需重复追加
	}
	add := dir
	if cur != "" {
		add = cur + ";" + dir
	}
	setCmd := exec.Command("powershell", "-NoProfile", "-Command",
		fmt.Sprintf("[Environment]::SetEnvironmentVariable('PATH',%s,'User')", psQuote(add)))
	if out2, err2 := setCmd.CombinedOutput(); err2 != nil {
		return fmt.Errorf("写入用户 PATH 失败: %v: %s", err2, strings.TrimSpace(string(out2)))
	}
	return nil
}

// pathContains 判断 Windows PATH（分号分隔）是否已包含 target（大小写不敏感）。
func pathContains(path, target string) bool {
	target = strings.ToLower(filepath.Clean(target))
	for _, p := range strings.Split(path, ";") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.ToLower(filepath.Clean(p)) == target {
			return true
		}
	}
	return false
}

// psQuote 为 PowerShell 单引号字符串转义（' -> ''）。
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
