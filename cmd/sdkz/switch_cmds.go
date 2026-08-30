package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// newDefaultCmd 生成 default 命令。
// Windows 上除更新 current 指针外，还会将 *_HOME 与 bin 写入用户级环境变量，
// 使未运行 sdkz init 的 PowerShell / CMD / Git Bash 也能生效（需新开终端）。
// 已运行 sdkz init 的 shell（bash/zsh/fish）会通过 sdkz() 函数 eval 导出的块即时生效。
func newDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default <candidate> <version>",
		Short: "设置全局默认版本（持久生效）",
		Long: `将版本设为全局默认（更新 current 指针），之后所有新开的终端自动使用该版本。

平台行为:
  - Windows: 自动写入用户级环境变量（JAVA_HOME / PATH 等），PowerShell / CMD / Git Bash
    无需 sdkz init 即可生效；已打开的终端需新开后才能看到变更（Windows 机制限制）。
  - Linux / macOS: 更新 current 软链；已 sdkz init 的 shell 即时生效，否则新终端生效。

参数:
  candidate  必填，支持的 SDK: java, go, node, maven, gradle
  version    必填，要设为默认的已安装版本

示例:
  sdkz default java 21.0.2`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			block, err := m.SetDefault(args[0], args[1])
			if err != nil {
				return err
			}
			// 供已 init 的 shell（bash/zsh/fish）sdkz() 函数 eval，即时生效。
			fmt.Fprint(cmd.OutOrStdout(), block)
			fmt.Fprintf(stderr(), "已将 %s 默认版本设为 %s\n", args[0], args[1])
			if runtime.GOOS == "windows" {
				fmt.Fprintln(stderr(), "Windows: 已写入用户级环境变量，请新开终端后生效（当前窗口可执行上面输出的导出语句立即生效）。")
			} else {
				fmt.Fprintln(stderr(), "新开终端或运行 'source <(~/.sdkz ...)' 后生效；已 init 的 shell 当前会话即时生效。")
			}
			return nil
		},
	}
}

func newCurrentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current [candidate]",
		Short: "显示当前 current 指向的版本",
		Long: `显示 current 指针指向的版本（即 default 设置的全局默认版本）。

参数 candidate 可省略：省略时列出全部 SDK 的 current 版本；指定时只显示该 SDK。
  支持的 candidate: java, go, node, maven, gradle

示例:
  sdkz current          # 列出全部 SDK 的当前版本
  sdkz current java     # 只显示 Java 当前版本`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				cur, err := m.Current(args[0])
				if err != nil {
					return err
				}
				if cur == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: 未设置当前版本\n", args[0])
					return nil
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", args[0], cur)
				return nil
			}
			for _, c := range m.Candidates() {
				cur, err := m.Current(c.ID)
				if err != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: 读取失败 (%v)\n", c.ID, err)
					continue
				}
				if cur == "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s: 未设置\n", c.ID)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", c.ID, cur)
			}
			return nil
		},
	}
}

func newHomeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "home <candidate> [version]",
		Short: "显示版本的安装目录",
		Long: `显示指定 SDK（及可选版本）在本地的安装目录绝对路径。
省略 version 时，显示 current 指向版本（或默认版本）的目录。

参数:
  candidate  必填，支持的 SDK: java, go, node, maven, gradle
  version    可选，具体版本；省略时取 current/默认版本

示例:
  sdkz home java            # 显示 Java 当前版本的安装目录
  sdkz home go 1.22.5       # 显示指定 Go 版本的安装目录`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			ver := ""
			if len(args) == 2 {
				ver = args[1]
			}
			dir, err := m.Home(args[0], ver)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), dir)
			return nil
		},
	}
	return cmd
}

func newEnvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "输出环境变量导出语句（供 eval）",
		Long: `输出当前所有已安装 SDK 的 PATH 与 *_HOME 环境变量导出语句。

用途:
  - 手动集成（未使用 sdkz init 时）: eval "$(sdkz env)"
  - CI 脚本中一次性注入 SDK 环境

注意: 该命令向 stdout 输出可 eval 的 shell 语句，请直接配合 eval 使用，
不要将其混入其他日志输出。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), m.EnvBlock())
			return nil
		},
	}
}
