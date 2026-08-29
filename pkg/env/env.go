// Package env 负责环境变量导出语句与 shell 集成。
package env

import (
	"fmt"
	"path/filepath"
	"strings"

	"sdkz/pkg/domain"
)

// ExportMarker 标记 eval 块的起始行（shell 函数据此判断是否 eval）。
const ExportMarker = "# SDKZ_EXPORT"

// Block 表示一组环境变量导出语句。
type Block struct {
	Shell string // bash / zsh / fish / pwsh
	Lines []string
}

// ExportBlock 生成将某候选的版本目录导出到环境的语句。
// dir 为版本目录（use 场景）或 current 目录（default 场景）。
func ExportBlock(c *domain.Candidate, dir, shell string) *Block {
	b := &Block{Shell: shell}
	if c.HomeEnv != "" {
		b.addAssign(c.HomeEnv, dir)
	}
	bin := filepath.Join(dir, c.BinDir)
	b.addPathPrefix(bin)
	return b
}

// ExportAll 生成所有候选的导出块（用于 `sdkz env`）。
// dirOf 返回某候选的当前目录。
func ExportAll(shell string, candidates []*domain.Candidate, dirOf func(*domain.Candidate) (string, bool)) *Block {
	b := &Block{Shell: shell}
	for _, c := range candidates {
		dir, ok := dirOf(c)
		if !ok {
			continue
		}
		if c.HomeEnv != "" {
			b.addAssign(c.HomeEnv, dir)
		}
		b.addPathPrefix(filepath.Join(dir, c.BinDir))
	}
	return b
}

func (b *Block) addAssign(key, value string) {
	switch b.Shell {
	case "fish":
		b.Lines = append(b.Lines, fmt.Sprintf("set -gx %s %q", key, value))
	case "pwsh":
		b.Lines = append(b.Lines, fmt.Sprintf("$env:%s = %q", key, value))
	default: // bash / zsh
		b.Lines = append(b.Lines, fmt.Sprintf("export %s=%q", key, value))
	}
}

func (b *Block) addPathPrefix(dir string) {
	switch b.Shell {
	case "fish":
		b.Lines = append(b.Lines, fmt.Sprintf("set -gx PATH %q $PATH", dir))
	case "pwsh":
		b.Lines = append(b.Lines, fmt.Sprintf("$env:PATH = %q + \";\" + $env:PATH", dir))
	default:
		b.Lines = append(b.Lines, fmt.Sprintf("export PATH=%q:$PATH", dir))
	}
}

// String 返回可 eval 的导出文本（首行为标记）。
func (b *Block) String() string {
	var sb strings.Builder
	sb.WriteString(ExportMarker)
	sb.WriteByte('\n')
	for _, l := range b.Lines {
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// Body 返回去掉标记行后的纯导出文本。
func (b *Block) Body() string {
	lines := b.Lines
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}
