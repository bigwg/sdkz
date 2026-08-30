package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"sdkz/pkg/domain"
)

func stderr() *os.File { return os.Stderr }

// isTTY 判断文件是否为终端。
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// progressPrinter 在 stderr 上打印安装/下载进度。
// TTY 下显示单行刷新进度条（百分比 + 字节 + 速度）；非 TTY（管道/CI）下
// 周期性打印一行文本进度，确保用户能感知任务在持续推进，不会误以为卡死。
type progressPrinter struct {
	mu          sync.Mutex
	stage       string
	printed     bool
	tty         bool
	barWidth    int
	lastDone    int64
	lastTime    time.Time
	lastLogTime time.Time
}

func newProgressPrinter() *progressPrinter {
	return &progressPrinter{tty: isTTY(os.Stderr), barWidth: 30}
}

// OnProgress 实现 service.ProgressFunc。
func (p *progressPrinter) OnProgress(done, total int64, stage string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 阶段切换：结束上一行，打印新阶段标题。
	if stage != "" && stage != p.stage {
		p.finishLine()
		p.stage = stage
		fmt.Fprintf(os.Stderr, "正在%s…\n", stage)
		p.lastDone = 0
		p.lastTime = time.Time{}
	}

	if done <= 0 || total <= 0 {
		return
	}

	now := time.Now()
	if p.lastTime.IsZero() {
		p.lastTime = now
		p.lastDone = done
		p.lastLogTime = now
	}

	if p.tty {
		p.renderBar(done, total, now)
	} else {
		p.renderLog(done, total, now)
	}

	if done >= total {
		p.finishLine()
	}
}

// renderBar 在 TTY 上以单行回车刷新进度条。
func (p *progressPrinter) renderBar(done, total int64, now time.Time) {
	pct := float64(done) / float64(total)
	filled := int(pct * float64(p.barWidth))
	if filled > p.barWidth {
		filled = p.barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.barWidth-filled)
	speed := p.calcSpeed(done, now)
	fmt.Fprintf(os.Stderr, "\r  [%s] %5.1f%%  %s / %s  (%s/s)",
		bar, pct*100, domain.FormatBytes(done), domain.FormatBytes(total), speed)
	p.printed = true
}

// renderLog 在非 TTY 下周期性打印一行进度（至少每 1s 一次）。
func (p *progressPrinter) renderLog(done, total int64, now time.Time) {
	if now.Sub(p.lastLogTime) < time.Second {
		return
	}
	p.lastLogTime = now
	pct := 100 * float64(done) / float64(total)
	speed := p.calcSpeed(done, now)
	fmt.Fprintf(os.Stderr, "  %s %5.1f%%  %s / %s  (%s/s)\n",
		p.stage, pct, domain.FormatBytes(done), domain.FormatBytes(total), speed)
}

// calcSpeed 估算瞬时下载速度（B/s），零值返回 "-"。
func (p *progressPrinter) calcSpeed(done int64, now time.Time) string {
	elapsed := now.Sub(p.lastTime).Seconds()
	if elapsed <= 0 {
		return "-"
	}
	bytesPerSec := float64(done-p.lastDone) / elapsed
	p.lastDone = done
	p.lastTime = now
	if bytesPerSec <= 0 {
		return "-"
	}
	return domain.FormatBytes(int64(bytesPerSec))
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
