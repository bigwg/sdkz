package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func newMirrorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "管理下载镜像（加速/国内访问）",
		Long: `管理下载镜像规则（config.toml 中的 mirror.<key>）。

镜像规则将下载 URL 中以 "https://<key>/" 开头的部分替换为指定地址
（key 可为 host 或 host/路径前缀，如 dl.google.com/go），仅作用于下载地址，
不影响版本清单 API。

常用子命令:
  sdkz mirror use cn       # 一键启用国内镜像（推荐国内用户）
  sdkz mirror add <key> <url>    # 手动添加镜像规则
  sdkz mirror list         # 列出当前生效的镜像规则
  sdkz mirror clear        # 清空所有镜像规则`,
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "use cn",
			Short: "启用国内镜像",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if args[0] != "cn" {
					return fmt.Errorf("目前仅提供预设 cn")
				}
				m, err := manager()
				if err != nil {
					return err
				}
				if err := m.Config().UseCN(); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "已启用国内镜像（node→npmmirror、go→goproxy.cn、gradle→tuna、maven→aliyun 等）")
				return nil
			},
		},
		&cobra.Command{
			Use:   "add <host> <url>",
			Short: "添加镜像规则",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				m, err := manager()
				if err != nil {
					return err
				}
				cfg := m.Config()
				if cfg.Mirror == nil {
					cfg.Mirror = map[string]string{}
				}
				cfg.Mirror[args[0]] = args[1]
				if err := cfg.Save(); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "已添加镜像: %s → %s\n", args[0], args[1])
				return nil
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "列出镜像规则",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				m, err := manager()
				if err != nil {
					return err
				}
				mirrors := m.Config().Mirror
				if len(mirrors) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "（无镜像规则）")
					return nil
				}
				hosts := make([]string, 0, len(mirrors))
				for h := range mirrors {
					hosts = append(hosts, h)
				}
				sort.Strings(hosts)
				for _, h := range hosts {
					fmt.Fprintf(cmd.OutOrStdout(), "  %s → %s\n", h, mirrors[h])
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "clear",
			Short: "清空镜像规则",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				m, err := manager()
				if err != nil {
					return err
				}
				if err := m.Config().ClearMirror(); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "已清空镜像规则")
				return nil
			},
		},
	)
	return cmd
}

func newCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "管理本地元数据缓存",
		Long: `管理本地缓存的远程元数据（版本清单）。

元数据首次拉取后会缓存到 ~/.sdkz/cache，之后 list 优先使用缓存并后台刷新。
网络异常时也会自动回退到缓存，避免完全不可用。

子命令:
  sdkz cache clean    # 清空所有缓存的元数据（下次 list 会重新联网拉取）`,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "clean",
		Short: "清空元数据缓存",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			if err := m.CleanCache(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "已清空元数据缓存")
			return nil
		},
	})
	return cmd
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "诊断环境问题（排错首选）",
		Long: `检查 sdkz 运行环境是否健康，列出各项状态。

会检查的内容包括:
  - 数据目录 ~/.sdkz 是否可写
  - 是否已执行 sdkz init（shell 集成是否就绪）
  - current 指针是否指向有效已安装版本
  - 镜像 / 离线等配置是否合理

当遇到版本不生效、命令找不到等问题时，先运行本命令定位原因。

示例:
  sdkz doctor`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			allOK := true
			for _, c := range m.Doctor() {
				mark := "[OK]  "
				if !c.OK {
					mark = "[FAIL]"
					allOK = false
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s: %s\n", mark, c.Name, c.Detail)
			}
			if allOK {
				fmt.Fprintln(cmd.OutOrStdout(), "环境正常")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "发现问题，请根据上方提示处理（多为 sdkz init 未执行）")
			}
			return nil
		},
	}
}
