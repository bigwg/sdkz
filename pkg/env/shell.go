package env

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DetectShell 从环境推断当前 shell。
// 返回 bash / zsh / fish / pwsh / powershell（Windows 5.1）/ unknown。
func DetectShell() string {
	if runtime.GOOS == "windows" {
		return "pwsh"
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		base := filepath.Base(sh)
		switch base {
		case "bash", "zsh", "fish", "pwsh", "powershell":
			return base
		}
		if strings.Contains(base, "zsh") {
			return "zsh"
		}
		return "bash"
	}
	return "unknown"
}

// DetectShellOr 返回 DetectShell，unknown 时用默认值。
func DetectShellOr(def string) string {
	s := DetectShell()
	if s == "unknown" {
		return def
	}
	return s
}

// RCPath 返回 shell 对应的配置文件路径。
func RCPath(shell string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	switch shell {
	case "bash":
		return filepath.Join(home, ".bashrc")
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish")
	case "pwsh":
		if runtime.GOOS == "windows" {
			return filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
		}
		return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1")
	case "powershell":
		return filepath.Join(home, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
	}
	return ""
}

const (
	markerBegin = "# >>> sdkz initialize >>>"
	markerEnd   = "# <<< sdkz initialize <<<"
)

// InitSnippet 返回指定 shell 的初始化块。
func InitSnippet(shell string) string {
	switch shell {
	case "fish":
		return fishSnippet()
	case "pwsh":
		return pwshSnippet()
	default: // bash / zsh
		return bashSnippet()
	}
}

func bashSnippet() string {
	return `# >>> sdkz initialize >>>
export SDKZ_DIR="${SDKZ_DIR:-$HOME/.sdkz}"
for c in java go node maven gradle; do
  d="$SDKZ_DIR/candidates/$c/current"
  if [ -d "$d/bin" ]; then
    case "$PATH:" in *"$d/bin:"*) ;; *) export PATH="$d/bin:$PATH" ;; esac
    case "$c" in
      java)  export JAVA_HOME="$d" ;;
      go)    export GOROOT="$d" ;;
      maven) export MAVEN_HOME="$d" ;;
      gradle) export GRADLE_HOME="$d" ;;
    esac
  fi
done
sdkz() {
  local out
  if out="$(command sdkz "$@")"; then
    case "$out" in
      '# SDKZ_EXPORT'*) printf '%s\n' "$out" | tail -n +2 | eval ;;
      *) printf '%s\n' "$out" ;;
    esac
  fi
}
# <<< sdkz initialize <<<`
}

func fishSnippet() string {
	return `# >>> sdkz initialize >>>
set -gx SDKZ_DIR "$HOME/.sdkz"
for c in java go node maven gradle
  set -l d "$SDKZ_DIR/candidates/$c/current"
  if test -d "$d/bin"
    if not contains -- "$d/bin" $PATH
      set -gx PATH "$d/bin" $PATH
    end
    switch $c
      case java
        set -gx JAVA_HOME "$d"
      case go
        set -gx GOROOT "$d"
      case maven
        set -gx MAVEN_HOME "$d"
      case gradle
        set -gx GRADLE_HOME "$d"
    end
  end
end
function sdkz
  set -l raw (command sdkz $argv 2>/dev/null)
  if test (count $raw) -ge 1; and string match -q '# SDKZ_EXPORT*' -- $raw[1]
    set -l body (string join \n $raw[2..-1])
    eval $body
  else
    printf '%s\n' $raw
  end
end
# <<< sdkz initialize <<<`
}

func pwshSnippet() string {
	return `# >>> sdkz initialize >>>
$env:SDKZ_DIR = if ($env:SDKZ_DIR) { $env:SDKZ_DIR } else { Join-Path $HOME ".sdkz" }
foreach ($c in @("java","go","node","maven","gradle")) {
  $d = Join-Path $env:SDKZ_DIR "candidates\$c\current"
  if (Test-Path (Join-Path $d "bin")) {
    $b = Join-Path $d "bin"
    if ($env:PATH -notlike "*$b*") { $env:PATH = "$b;$env:PATH" }
    switch ($c) {
      "java"  { $env:JAVA_HOME = $d }
      "go"    { $env:GOROOT = $d }
      "maven" { $env:MAVEN_HOME = $d }
      "gradle" { $env:GRADLE_HOME = $d }
    }
  }
}
function global:sdkz {
  $out = & (Get-Command sdkz -CommandType Application) @args 2>&1
  $lines = if ($out -is [array]) { $out } else { @("$out") }
  # 在输出中定位 # SDKZ_EXPORT 标记行（即使前面混入诊断信息也能找到）。
  $idx = -1
  for ($i = 0; $i -lt $lines.Count; $i++) {
    if ($lines[$i] -match '^# SDKZ_EXPORT') { $idx = $i; break }
  }
  if ($idx -ge 0) {
    # 逐行作为 PowerShell 语句执行（与 bash 的 eval 等价），
    # 由 PowerShell 原生处理反斜杠路径与 $env:PATH 展开。
    $lines | Select-Object -Skip ($idx + 1) | ForEach-Object { Invoke-Expression $_ }
  } else {
    $lines | ForEach-Object { Write-Output $_ }
  }
}
# <<< sdkz initialize <<<`
}

// Inject 幂等地将初始化块写入 shell 配置文件。
// 已存在时整体替换标记区间。返回写入的文件路径。
func Inject(shell string) (string, error) {
	rc := RCPath(shell)
	if rc == "" {
		return "", fmt.Errorf("不支持的 shell: %s", shell)
	}
	snippet := InitSnippet(shell)
	content := ""
	if data, err := os.ReadFile(rc); err == nil {
		content = string(data)
	}
	newContent := replaceBlock(content, snippet)
	if newContent == content {
		// 无变化也确保文件存在（内容为空时）。
		newContent = content + snippet
	}
	if err := os.MkdirAll(filepath.Dir(rc), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(rc, []byte(newContent), 0o644); err != nil {
		return "", err
	}
	return rc, nil
}

// replaceBlock 替换文本中的标记区间；无标记则追加。
func replaceBlock(content, snippet string) string {
	start := strings.Index(content, markerBegin)
	end := strings.Index(content, markerEnd)
	if start < 0 || end < 0 || end < start {
		if strings.TrimSpace(content) == "" {
			return snippet + "\n"
		}
		return content + "\n" + snippet + "\n"
	}
	end += len(markerEnd)
	// 去掉标记块尾随换行。
	for end < len(content) && (content[end] == '\n' || content[end] == '\r') {
		end++
	}
	return content[:start] + snippet + "\n" + content[end:]
}

// IsInjected 判断配置文件是否已注入初始化块。
func IsInjected(shell string) bool {
	rc := RCPath(shell)
	if rc == "" {
		return false
	}
	data, err := os.ReadFile(rc)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), markerBegin)
}
