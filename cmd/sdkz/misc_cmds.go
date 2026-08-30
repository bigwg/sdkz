package main

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"sdkz/pkg/env"
	"sdkz/pkg/selfupdate"
	"sdkz/pkg/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示版本信息",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version.String())
			return nil
		},
	}
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [shell]",
		Short: "注入 shell 集成（写入 rc 配置文件）",
		Long: `将 sdkz 初始化块写入你的 shell 配置文件，使 sdkz 管理的 PATH 与
*_HOME 环境变量自动生效，并支持 default 在当前终端立即生效（已 init 的 shell）。
Windows 上 default 还会写入用户级环境变量，无需 init 即可在 PowerShell/CMD/Git Bash 生效。

支持的 shell（参数可填其一）:
  bash    写入 ~/.bashrc
  zsh     写入 ~/.zshrc
  fish    写入 ~/.config/fish/config.fish
  pwsh    PowerShell 7+，写入 Microsoft.PowerShell_profile.ps1
  powershell  Windows PowerShell 5.1，写入 WindowsPowerShell profile

省略 shell 时自动探测（依据 $SHELL 环境变量；Windows 默认用 pwsh）。

说明:
  - 注入内容用标记包裹，可重复运行，不会重复追加
  - 安装脚本（install.sh / install.ps1）已自动执行本命令

示例:
  sdkz init          # 自动探测
  sdkz init zsh      # 显式指定`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			shell := env.DetectShellOr("bash")
			if len(args) == 1 {
				shell = args[0]
			}
			switch shell {
			case "bash", "zsh", "fish", "pwsh", "powershell":
			default:
				return fmt.Errorf("不支持的 shell: %s（支持 bash/zsh/fish/pwsh）", shell)
			}
			rc, err := m.Init(shell)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "已写入 %s\n", rc)
			fmt.Fprintf(cmd.OutOrStdout(), "请重新打开终端，或执行 source %s 使配置立即生效\n", rc)
			return nil
		},
	}
}

func newSelfUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "selfupdate",
		Short: "从 GitHub Releases 升级 sdkz 自身",
		Long: `从 GitHub Releases 下载最新版二进制替换当前可执行文件，升级 sdkz 本身。

前提: 配置文件中需设置 self_update_repo（默认已指向 bigwg/sdkz）。
下载前会校验 checksums.txt 中的 sha256，并通过临时文件原子替换，保证安全。

说明: 安装脚本首次安装即包含此能力，之后无需重跑安装脚本。

示例:
  sdkz selfupdate`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			repo := m.Config().SelfUpdateRepo
			if repo == "" {
				// 未显式配置时，默认使用本项目仓库。
				repo = "bigwg/sdkz"
			}
			p := newProgressPrinter()
			ver, err := selfupdate.Update(cmd.Context(), httpClient(), repo, p.OnProgress)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "已更新到 %s\n", ver)
			return nil
		},
	}
}

// httpClient 供 selfupdate 等场景使用。
func httpClient() *http.Client {
	return &http.Client{}
}
