package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"sdkz/pkg/domain"
	"sdkz/pkg/installer"
)

func newInstallCmd() *cobra.Command {
	var vendor string
	cmd := &cobra.Command{
		Use:   "install <candidate> [version]",
		Short: "安装指定 SDK 版本",
		Long: `安装指定 SDK 的指定版本到数据目录（默认 ~/.sdkz）。

参数:
  candidate  必填，支持的 SDK: java, go, node, maven, gradle
  version    可选，省略时安装该 SDK 的默认版本
             版本规格支持:
               latest      最新稳定版（go/node 默认）
               lts         LTS 版本（node/java 适用）
               21          主版本号，取该主版本最新小版本
               21.0.2     精确版本号
               21.0.5+11  带构建号的精确版本（容忍 + 后缀）

发行版（仅 java）: 用 --vendor 指定 tem(Temurin,默认)/zul/graal

示例:
  sdkz install java 21              # 安装 Java 21 主版本最新小版本（默认 Temurin）
  sdkz install java 21.0.2          # 安装精确版本
  sdkz install java --vendor zul 21 # 安装 Azul Zulu 的 Java 21
  sdkz install go                   # 安装 Go 最新稳定版
  sdkz install node lts             # 安装 Node.js 最新 LTS`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			candID := args[0]
			spec := ""
			if len(args) == 2 {
				spec = args[1]
			}
			cand, err := m.FindCandidate(candID)
			if err != nil {
				return err
			}
			p := newProgressPrinter()
			choose := func(cands []*domain.Release) (int, error) {
				return promptChoice(cand, cands)
			}
			rel, err := m.Install(cmd.Context(), candID, spec, vendor, choose, p.OnProgress)
			if err != nil {
				if errors.Is(err, installer.ErrAlreadyInstalled) {
					return fmt.Errorf("%s %s 已安装（可用 sdkz default 切换为全局默认）", candID, rel.Version)
					}
					return err
					}
					fmt.Fprintf(cmd.OutOrStdout(), "已安装 %s %s\n", candID, rel.Version)
					fmt.Fprintf(cmd.OutOrStdout(), "运行 sdkz default %s %s 设为全局默认（Windows 需新开终端生效）\n",
					candID, rel.Version)
					return nil
		},
	}
	cmd.Flags().StringVar(&vendor, "vendor", "", "指定发行版（如 tem/zul/graal）")
	return cmd
}

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <candidate> <version>",
		Short: "卸载指定 SDK 版本",
		Long: `从数据目录中删除指定的已安装 SDK 版本。
若卸载的是 current 指向的版本，current 指针将一并清除。

参数:
  candidate  必填，支持的 SDK: java, go, node, maven, gradle
  version    必填，要卸载的具体版本（可用 "sdkz ls" 查看已安装版本）

示例:
  sdkz uninstall java 21.0.2
  sdkz uninstall go 1.22.5`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			if err := m.Uninstall(args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "已卸载 %s %s\n", args[0], args[1])
			return nil
		},
	}
}
