package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStderr 临时替换 os.Stderr 并收集输出。
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	w.Close()
	data, _ := io.ReadAll(r)
	return string(data)
}

// TestProgressPrinterNoDuplicateFinish 验证 100% 完成后，同阶段的重复进度回调
// 不会再次渲染（防止出现两条进度条）。
func TestProgressPrinterNoDuplicateFinish(t *testing.T) {
	p := &progressPrinter{tty: true, barWidth: 10}

	out := captureStderr(t, func() {
		p.OnProgress(50, 100, "更新下载")  // 渲染 50%
		p.OnProgress(100, 100, "更新下载") // 渲染 100% 并换行
		p.OnProgress(100, 100, "更新下载") // 重复回调，应被忽略
	})

	if n := strings.Count(out, "100.0%"); n != 1 {
		t.Fatalf("100%% 应只渲染一次，实际 %d 次:\n%s", n, out)
	}
	// 阶段切换应重置防抖，新阶段可正常渲染。
	out = captureStderr(t, func() {
		p.OnProgress(0, 0, "校验")
		p.OnProgress(30, 60, "校验")
	})
	if !strings.Contains(out, "正在校验…") {
		t.Fatalf("阶段切换应打印新标题:\n%s", out)
	}
	if !strings.Contains(out, "50.0%") {
		t.Fatalf("新阶段应正常渲染进度:\n%s", out)
	}
}
