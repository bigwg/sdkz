package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"sdkz/pkg/domain"
	"sdkz/pkg/env"
	"sdkz/pkg/installer"
	"sdkz/pkg/linker"
)

// ErrNeedChoice 表示需要用户在多个发行版中做出选择。
var ErrNeedChoice = errors.New("需要选择发行版")

// ListRemote 返回候选的可安装版本（已缓存/离线回退）。
func (m *Manager) ListRemote(ctx context.Context, candID, vendorID string) ([]*domain.Release, error) {
	cand, err := m.FindCandidate(candID)
	if err != nil {
		return nil, err
	}
	return m.cat.ListRemote(ctx, cand, vendorID)
}

// ListInstalled 返回候选已安装的版本目录名。
func (m *Manager) ListInstalled(candID string) ([]string, error) {
	if _, err := m.FindCandidate(candID); err != nil {
		return nil, err
	}
	return m.ins.Installed(candID)
}

// Current 返回候选当前使用的版本名；未设置返回空串。
func (m *Manager) Current(candID string) (string, error) {
	cur, err := m.link.Current(candID)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return cur, nil
}

// MatchReleases 返回匹配版本规格的候选发行版（各 vendor 取最优一个）。
// spec 为空时使用候选默认规格；支持 latest / lts / 21 / 21.0 / 21.0.5。
func (m *Manager) MatchReleases(ctx context.Context, cand *domain.Candidate, spec, vendorID string) ([]*domain.Release, error) {
	rels, err := m.cat.ListRemote(ctx, cand, vendorID)
	if err != nil {
		return nil, err
	}
	if len(rels) == 0 {
		return nil, fmt.Errorf("候选 %s 当前没有可用版本", cand.ID)
	}
	spec = strings.TrimSpace(spec)
	if spec == "" {
		spec = cand.Default
	}
	if spec == "lts" {
		var lts []*domain.Release
		for _, r := range rels {
			if r.LTS {
				lts = append(lts, r)
			}
		}
		if len(lts) > 0 {
			rels = lts
		} else {
			spec = "latest"
		}
	}

	// 按 vendor 分组，各取最优。
	byVendor := map[string]*domain.Release{}
	for _, r := range rels {
		if spec != "latest" {
			if !matchesSpec(spec, r.Version) {
				continue
			}
		}
		best, ok := byVendor[r.VendorID]
		if !ok || betterRelease(r, best) {
			byVendor[r.VendorID] = r
		}
	}
	out := make([]*domain.Release, 0, len(byVendor))
	for _, r := range byVendor {
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("没有找到匹配 %q 的版本（候选 %s）", spec, cand.ID)
	}
	sortByVendorOrder(cand, out)
	return out, nil
}

// matchesSpec 判断单版本是否满足规格。
func matchesSpec(spec, v string) bool {
	got, err := domain.MatchVersion(spec, []string{v})
	return err == nil && got == v
}

// betterRelease 判断 a 是否优于 b（GA 优先，再按版本数值）。
func betterRelease(a, b *domain.Release) bool {
	if a.Stable != b.Stable {
		return a.Stable
	}
	return domain.MustParse(a.Version).Compare(domain.MustParse(b.Version)) > 0
}

func sortByVendorOrder(cand *domain.Candidate, out []*domain.Release) {
	order := map[string]int{}
	for i, v := range cand.Vendors {
		order[v.ID] = i
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if order[out[j].VendorID] < order[out[i].VendorID] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
}

// Install 安装匹配的版本。choose 为空时自动选择（默认 vendor 优先）。
// 返回安装的发行版信息。
func (m *Manager) Install(ctx context.Context, candID, versionSpec, vendorID string, choose ChooseFunc, prog ProgressFunc) (*domain.Release, error) {
	cand, err := m.FindCandidate(candID)
	if err != nil {
		return nil, err
	}
	cands, err := m.MatchReleases(ctx, cand, versionSpec, vendorID)
	if err != nil {
		return nil, err
	}
	idx := -1
	if len(cands) > 1 {
		if choose != nil {
			idx, err = choose(cands)
			if err != nil {
				return nil, err
			}
		} else {
			idx = defaultChoice(cand, cands)
		}
	} else {
		idx = 0
	}
	if idx < 0 || idx >= len(cands) {
		return nil, ErrNeedChoice
	}
	rel := cands[idx]

	if m.ins.IsInstalled(candID, rel.Version) {
		return rel, fmt.Errorf("%s %s 已安装: %w", candID, rel.Version, installer.ErrAlreadyInstalled)
	}
	if _, err := m.ins.Install(ctx, rel, prog); err != nil {
		return nil, err
	}
	return rel, nil
}

func defaultChoice(cand *domain.Candidate, cands []*domain.Release) int {
	if cand.DefaultVendor != "" {
		for i, r := range cands {
			if r.VendorID == cand.DefaultVendor {
				return i
			}
		}
	}
	return 0
}

// Uninstall 卸载版本。
func (m *Manager) Uninstall(candID, ver string) error {
	if _, err := m.FindCandidate(candID); err != nil {
		return err
	}
	if cur, err := m.Current(candID); err == nil && cur == ver {
		return fmt.Errorf("不能卸载当前使用中的版本 %s %s，请先切换", candID, ver)
	}
	if err := m.ins.Uninstall(candID, ver); err != nil {
		return err
	}
	m.ins.CleanStale()
	return nil
}

// SetDefault 将版本设为全局默认（更新 current 指针），并返回可 eval 的 export 块。
// Windows 上还会将 *_HOME 与 bin 路径持久写入用户级环境变量（HKCU\Environment），
// 使得即使未运行 sdkz init、无 shell 集成，PowerShell / CMD / Git Bash 也能生效。
func (m *Manager) SetDefault(candID, ver string) (string, error) {
	cand, err := m.FindCandidate(candID)
	if err != nil {
		return "", err
	}
	if !m.ins.IsInstalled(candID, ver) {
		return "", fmt.Errorf("%s %s 未安装，请先运行 sdkz install", candID, ver)
	}
	dir := m.InstalledDir(candID, ver)
	if _, err := m.link.Set(candID, dir); err != nil {
		return "", err
	}
	curDir := m.InstalledDir(candID, "current")
	binDir := filepath.Join(curDir, cand.BinDir)
	if runtime.GOOS == "windows" {
		if err := linker.SetUserEnvWindows(linker.UserEnv{
			HomeEnv: cand.HomeEnv,
			HomeDir: curDir,
			BinDir:  binDir,
		}); err != nil {
			// 用户级环境变量写入失败不应阻断 default，仅告警。
			warnf("写入用户级环境变量失败（不影响指针切换）: %v", err)
		}
	}
	block := env.ExportBlock(cand, curDir, m.Shell)
	return block.String(), nil
}

// EnvBlock 输出全部已安装候选的环境导出块。
func (m *Manager) EnvBlock() string {
	block := env.ExportAll(m.Shell, m.Candidates(), func(c *domain.Candidate) (string, bool) {
		cur, err := m.Current(c.ID)
		if err != nil || cur == "" {
			return "", false
		}
		return m.InstalledDir(c.ID, "current"), true
	})
	return block.String()
}

// warnf 输出非致命告警到 stderr（Windows 用户级环境变量写入失败时软降级）。
func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "警告: "+format+"\n", args...)
}

// Home 返回版本目录路径（ver 为空返回 current 目录）。
func (m *Manager) Home(candID, ver string) (string, error) {
	if _, err := m.FindCandidate(candID); err != nil {
		return "", err
	}
	if ver == "" {
		cur, err := m.Current(candID)
		if err != nil || cur == "" {
			return "", fmt.Errorf("%s 未设置当前版本", candID)
		}
		return m.InstalledDir(candID, "current"), nil
	}
	if !m.ins.IsInstalled(candID, ver) {
		return "", fmt.Errorf("%s %s 未安装", candID, ver)
	}
	return m.InstalledDir(candID, ver), nil
}

// Which 返回当前版本的 bin 可执行目录路径。
func (m *Manager) Which(candID string) (string, error) {
	cand, err := m.FindCandidate(candID)
	if err != nil {
		return "", err
	}
	home, err := m.Home(candID, "")
	if err != nil {
		return "", err
	}
	return home + string(os.PathSeparator) + cand.BinDir, nil
}

// Init 注入 shell 集成，返回配置文件路径。
func (m *Manager) Init(shell string) (string, error) {
	return env.Inject(shell)
}

// IsInjected 检查 shell 是否已集成。
func (m *Manager) IsInjected(shell string) bool { return env.IsInjected(shell) }

// CleanCache 清空元数据缓存。
func (m *Manager) CleanCache() error {
	dir := m.cfg.Root + string(os.PathSeparator) + "metadata"
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if err := os.Remove(dir + string(os.PathSeparator) + e.Name()); err != nil {
			return err
		}
	}
	return nil
}

// CleanStale 清理下载/解压临时文件。
func (m *Manager) CleanStale() {
	m.ins.CleanStale()
}
