package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"

	"sdkz/pkg/domain"
)

func stderr() *os.File { return os.Stderr }

// isTTY 判断文件是否为终端。
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// progressPrinter 在 stderr 上打印安装进度。
type progressPrinter struct {
	mu      sync.Mutex
	stage   string
	printed bool
	tty     bool
}

func newProgressPrinter() *progressPrinter {
	return &progressPrinter{tty: isTTY(os.Stderr)}
}

// OnProgress 实现 service.ProgressFunc。
func (p *progressPrinter) OnProgress(done, total int64, stage string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if stage != "" && stage != p.stage {
		p.finishLine()
		p.stage = stage
		fmt.Fprintf(os.Stderr, "正在%s…\n", stage)
	}
	if done > 0 && total > 0 {
		if p.tty {
			pct := 100 * float64(done) / float64(total)
			fmt.Fprintf(os.Stderr, "\r  %6.1f%%  %s / %s", pct, domain.FormatBytes(done), domain.FormatBytes(total))
			p.printed = true
		}
		if done >= total {
			p.finishLine()
		}
	}
}

func (p *progressPrinter) finishLine() {
	if p.printed {
		fmt.Fprintln(os.Stderr)
		p.printed = false
	}
}

// promptChoice 交互选择发行版；非终端时自动选默认。
func promptChoice(cand *domain.Candidate, cands []*domain.Release) (int, error) {
	if !isTTY(os.Stdin) || !isTTY(os.Stderr) {
		return defaultIndex(cand, cands), nil
	}
	fmt.Fprintf(os.Stderr, "%s 存在多个发行版，请选择：\n", cand.Name)
	for i, r := range cands {
		fmt.Fprintf(os.Stderr, "  [%d] %s (%s)\n", i+1, r.Version, r.VendorName)
	}
	fmt.Fprint(os.Stderr, "请输入序号 [1]: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return 0, nil
	}
	var n int
	if _, err := fmt.Sscan(line, &n); err != nil || n < 1 || n > len(cands) {
		return 0, fmt.Errorf("无效选择: %s", line)
	}
	return n - 1, nil
}

// defaultIndex 返回默认发行版索引（默认 vendor 优先）。
func defaultIndex(cand *domain.Candidate, cands []*domain.Release) int {
	if cand.DefaultVendor != "" {
		for i, r := range cands {
			if r.VendorID == cand.DefaultVendor {
				return i
			}
		}
	}
	return 0
}
