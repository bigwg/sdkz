package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Check 表示一次诊断结果。
type Check struct {
	Name   string
	OK     bool
	Detail string
}

// Doctor 返回环境诊断结果列表。
func (m *Manager) Doctor() []Check {
	var out []Check

	// 1. 数据目录可写。
	if err := os.MkdirAll(m.cfg.Root, 0o755); err != nil {
		out = append(out, Check{Name: "数据目录", Detail: fmt.Sprintf("%s 不可创建: %v", m.cfg.Root, err)})
	} else {
		probe := filepath.Join(m.cfg.Root, ".write-test")
		if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
			out = append(out, Check{Name: "数据目录", Detail: fmt.Sprintf("%s 不可写: %v", m.cfg.Root, err)})
		} else {
			os.Remove(probe)
			out = append(out, Check{Name: "数据目录", OK: true, Detail: m.cfg.Root})
		}
	}

	// 2. shell 集成。
	if m.IsInjected(m.Shell) {
		out = append(out, Check{Name: "shell 集成", OK: true, Detail: m.Shell})
	} else {
		out = append(out, Check{Name: "shell 集成", Detail: "未注入，请运行 sdkz init " + m.Shell})
	}

	// 3. 平台支持。
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		out = append(out, Check{Name: "平台", Detail: "当前平台 " + runtime.GOOS + " 支持有限"})
	} else {
		out = append(out, Check{Name: "平台", OK: true, Detail: runtime.GOOS + "-" + runtime.GOARCH})
	}

	// 4. 各候选 current 指针健康。
	for _, c := range m.Candidates() {
		cur, err := m.Current(c.ID)
		if err != nil {
			out = append(out, Check{Name: c.ID + " 指针", Detail: "读取失败: " + err.Error()})
			continue
		}
		if cur == "" {
			out = append(out, Check{Name: c.ID, Detail: "未设置当前版本"})
			continue
		}
		if m.ins.IsInstalled(c.ID, cur) {
			out = append(out, Check{Name: c.ID, OK: true, Detail: cur})
		} else {
			out = append(out, Check{Name: c.ID, Detail: fmt.Sprintf("current 指向 %s 但目录不存在", cur)})
		}
	}

	return out
}
