package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"sdkz/pkg/service"
	"sdkz/pkg/version"
)

var (
	verbose bool
	_mgr    *service.Manager
)

// manager 惰性构造全局 Manager。
func manager() (*service.Manager, error) {
	if _mgr != nil {
		return _mgr, nil
	}
	m, err := service.New()
	if err != nil {
		return nil, err
	}
	_mgr = m
	return m, nil
}

var rootCmd = &cobra.Command{
	Use:   "sdkz",
	Short: "跨平台 SDK 版本管理工具",
	Long: `sdkz 是一个跨平台的 SDK 版本管理工具（对标 SDKMAN）。

支持的 SDK（candidate）：
  java    Java (JDK)        发行版: tem(Temurin,默认) / zul(Zulu) / graal(GraalVM)
  go      Go                官方发行版
  node    Node.js           官方发行版
  maven   Apache Maven      官方发行版
  gradle  Gradle            官方发行版

支持的操作系统: Linux / macOS / Windows（amd64 与 arm64）。
支持的 shell: bash / zsh / fish / pwsh(powershell)。

数据目录: ~/.sdkz（可用环境变量 SDKZ_DIR 覆盖）。
首次使用请先运行 "sdkz init" 完成 shell 集成。`,
	SilenceUsage:  true,
	SilenceErrors: false,
	Version:       version.String(),
}

func init() {
	rootCmd.SetVersionTemplate(version.String() + "\n")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "显示详细日志")
	rootCmd.AddCommand(
		newVersionCmd(),
		newInitCmd(),
		newListCmd(),
		newLsCmd(),
		newInstallCmd(),
		newUninstallCmd(),
		newDefaultCmd(),
		newCurrentCmd(),
		newHomeCmd(),
		newEnvCmd(),
		newUpgradeCmd(),
		newOfflineCmd(),
		newMirrorCmd(),
		newCacheCmd(),
		newDoctorCmd(),
		newSelfUpdateCmd(),
	)
}

// Execute 运行根命令。
func Execute() error {
	return rootCmd.Execute()
}

// warnf 输出非致命告警到 stderr。
func warnf(format string, args ...any) {
	fmt.Fprintf(stderr(), format+"\n", args...)
}
