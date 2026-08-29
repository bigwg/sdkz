// Package platform 负责操作系统与架构的归一化，以及跨平台能力探测。
package platform

import "runtime"

// Platform 表示目标运行平台。
type Platform struct {
	OS   string // linux / darwin / windows
	Arch string // amd64 / arm64
}

// Detect 返回当前运行平台。
func Detect() Platform {
	return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// FromParts 构造平台。
func FromParts(os, arch string) Platform { return Platform{OS: os, Arch: arch} }

func (p Platform) IsWindows() bool { return p.OS == "windows" }
func (p Platform) IsDarwin() bool  { return p.OS == "darwin" }
func (p Platform) IsLinux() bool   { return p.OS == "linux" }

// String 返回 "linux-amd64" 形式的标识。
func (p Platform) String() string { return p.OS + "-" + p.Arch }

// ArchiveExt 返回当前平台默认的归档扩展名。
func (p Platform) ArchiveExt() string {
	if p.IsWindows() {
		return "zip"
	}
	return "tar.gz"
}

// SupportedArch 判断架构是否在支持范围内（amd64/arm64）。
func (p Platform) Supported() bool {
	switch p.Arch {
	case "amd64", "arm64":
		return true
	}
	return false
}
