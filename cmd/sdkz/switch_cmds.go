package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newUseCmd 生成 use 命令。
// 重要：stdout 输出的是可被 shell 函数 eval 的 export 块（带标记行），
// 因此任何附加信息必须走 stderr。
func newUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <candidate> <version>",
		Short: "切换版本（仅当前 shell 会话生效）",
		Long: `将当前 shell 会话切换到指定已安装版本，仅对当前终端生效，退出终端后恢复。

前提:
  1. 已运行 "sdkz init" 完成 shell 集成（由 sdkz() 包装函数执行导出的环境变量）
  2. 目标版本已安装（用 "sdkz ls" 查看）

参数:
  candidate  必填，支持的 SDK: java, go, node, maven, gradle
  version    必填，要切换到的已安装版本

示例:
  sdkz use java 21.0.2
  sdkz use node 20`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			block, err := m.Use(args[0], args[1])
			if err != nil {
				return err
			}
			// 供 shell 函数 eval。
			fmt.Fprint(cmd.OutOrStdout(), block)
			return nil
		},
	}
}

func newDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default <candidate> <version>",
		Short: "设置全局默认版本（新终端自动生效）",
		Long: `将版本设为全局默认（更新 current 指针），之后所有新开的终端自动使用该版本。

与 use 的区别:
  default  持久化，影响 future 新终端（写入 current 指针）
  use      仅当前 shell 会话临时切换

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
			fmt.Fprint(cmd.OutOrStdout(), block)
			fmt.Fprintf(stderr(), "已将 %s 默认版本设为 %s（新终端生效）\n", args[0], args[1])
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
