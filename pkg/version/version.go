// Package version 提供版本信息（通过 ldflags 注入）。
package version

import "fmt"

// Version 由构建时注入：-ldflags "-X sdkz/pkg/version.Version=..."
var Version = "dev"

// Commit 由构建时注入（可选）。
var Commit = ""

func String() string {
	if Commit != "" {
		return fmt.Sprintf("sdkz %s (%s)", Version, Commit)
	}
	return fmt.Sprintf("sdkz %s", Version)
}
