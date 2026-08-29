package domain

import (
	"fmt"
	"strings"

	"sdkz/pkg/platform"
)

// Artifact 表示某一平台下的下载产物。
type Artifact struct {
	URL          string   // 主下载地址
	FallbackURLs []string // 主地址失败时依次尝试
	ChecksumURL  string   // 校验值文件地址（.sha256/.sha512/SHASUMS256.txt）
	ChecksumType string   // sha256 | sha512
	SHA256       string   // 内联校验值（优先于 ChecksumURL）
	Ext          string   // tar.gz | zip
	Strip        int      // 解压时剥掉的顶层目录数
	OriginalHost string   // 原始主机名（镜像替换用，构建时填充）
}

// ArchiveExt 推断产物扩展名。
func (a *Artifact) ArchiveExt() string {
	if a.Ext != "" {
		return a.Ext
	}
	return "tar.gz"
}

// Release 表示一个可安装的发行版本（对应特定平台）。
type Release struct {
	CandidateID string
	VendorID    string
	Version     string // 显示名，如 "21.0.5-tem"、"go1.23.4"、"v22.11.0"
	Artifact    *Artifact
	LTS         bool   // 是否 LTS（Node 适用）
	Stable      bool   // 是否 GA
	VendorName  string // 发行版展示名（展示用）
}

// Key 返回唯一键（候选+发行版+版本）。
func (r *Release) Key() string { return r.VendorID + "/" + r.Version }

// DirName 返回在 candidates 目录中的目录名（即 Version 本身）。
func (r *Release) DirName() string { return r.Version }

// PlatformUnsupportedError 表示当前平台没有对应产物。
type PlatformUnsupportedError struct{ C string }

func (e *PlatformUnsupportedError) Error() string {
	return fmt.Sprintf("候选 %s 暂不支持当前平台（%s）", e.C, platform.Detect().String())
}

// FormatBytes 格式化字节数（展示用）。
func FormatBytes(n int64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.2f GB", float64(n)/GB)
	case n >= MB:
		return fmt.Sprintf("%.2f MB", float64(n)/MB)
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/KB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// IsArchivePath 判断文件名是否为已知归档格式。
func IsArchivePath(name string) bool {
	return strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") ||
		strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar")
}
