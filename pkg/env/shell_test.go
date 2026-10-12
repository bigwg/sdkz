package env

import (
	"strings"
	"testing"
)

// 模拟用户损坏的 PowerShell profile：三段 sdkz 块，其中中间一段结尾与下一段开头粘连，且含乱码。
const damaged = `# >>> sdkz initialize >>>
old block 1
# <<< sdkz initialize <<<
# >>> sdkz initialize >>>
old block 2 (end glued to next begin)# >>> sdkz initialize >>>
old block 3
# <<< sdkz initialize <<<

user own content line
`

func TestRemoveAllBlocksConvergesToOne(t *testing.T) {
	clean := removeAllBlocks(damaged)
	if strings.Contains(clean, markerBegin) {
		t.Fatalf("removeAllBlocks 未清除所有标记块，残留:\n%s", clean)
	}
	if !strings.Contains(clean, "user own content line") {
		t.Fatalf("用户内容被误删:\n%s", clean)
	}
}

// 验证 Inject 的内部逻辑（removeAllBlocks + 追加一份）连续两次收敛为恰好一份。
func TestInjectConvergesToSingleBlock(t *testing.T) {
	snippet := pwshSnippet("/home/user/.local/bin")
	// 第一次：损坏内容 -> 删所有 + 追加一份。
	step1 := removeAllBlocks(damaged) + "\n" + snippet + "\n"
	if c := strings.Count(step1, markerBegin); c != 1 {
		t.Fatalf("第一次应为 1 个块，实际 %d", c)
	}
	// 第二次：对已收敛内容再执行一次。
	step2 := removeAllBlocks(step1) + "\n" + snippet + "\n"
	if c := strings.Count(step2, markerBegin); c != 1 {
		t.Fatalf("重复执行后应为 1 个块，实际 %d:\n%s", c, step2)
	}
	if !strings.Contains(step2, "user own content line") {
		t.Fatalf("用户内容丢失:\n%s", step2)
	}
}

// 验证 bash/zsh 包装函数的 eval 写法：必须是 eval "$(...)" 在当前 shell 执行；
// 管道写法（tail | eval）会使 eval 落在子 shell 中，export 全部丢失（历史 bug）。
func TestBashSnippetEvalInCurrentShell(t *testing.T) {
	s := InitSnippet("bash", "/home/u/.local/bin")
	if !strings.Contains(s, `eval "$(printf`) {
		t.Fatalf("bash 片段应使用 eval \"$(...)\" 在当前 shell 执行:\n%s", s)
	}
	if strings.Contains(s, "| eval") {
		t.Fatalf("bash 片段不得使用管道 eval（子 shell 中 export 会丢失）:\n%s", s)
	}
	if !strings.Contains(s, "SDKZ_SHELL_INTEGRATION") {
		t.Fatalf("bash 片段应注入集成标记环境变量:\n%s", s)
	}
	if f := InitSnippet("fish", "/home/u/.local/bin"); !strings.Contains(f, "SDKZ_SHELL_INTEGRATION") {
		t.Fatalf("fish 片段应注入集成标记环境变量:\n%s", f)
	}
	if w := InitSnippet("pwsh", "/home/u/.local/bin"); !strings.Contains(w, "SDKZ_SHELL_INTEGRATION") {
		t.Fatalf("pwsh 片段应注入集成标记环境变量:\n%s", w)
	}
}

func TestRemoveAllBlocksHalfBlock(t *testing.T) { // 真实场景：sdkz 块总是追加到文件末尾，半块（无 end）出现在末尾，用户内容在前。
	half := "before user content\n# >>> sdkz initialize >>>\norphan content without end\n"
	clean := removeAllBlocks(half)
	if strings.Contains(clean, markerBegin) || strings.Contains(clean, "orphan content") {
		t.Fatalf("半块未清除:\n%s", clean)
	}
	if !strings.Contains(clean, "before user content") {
		t.Fatalf("用户内容丢失:\n%s", clean)
	}
}
