package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
	"sdkz/pkg/service"
)

// 颜色（仅在交互式终端启用）。
const (
	cReset  = "\033[0m"
	cGreen  = "\033[32m"
	cCyan   = "\033[36m"
	cYellow = "\033[33m"
	cBold   = "\033[1m"
)

func colorStr(on bool, code, s string) string {
	if !on {
		return s
	}
	return code + s + cReset
}

func asFile(w io.Writer) *os.File {
	if f, ok := w.(*os.File); ok {
		return f
	}
	return nil
}

// isLTS 仅对 Java 按主版本判断 LTS（8/11/17/21）。
func isLTS(candID, ver string) bool {
	if candID != "java" {
		return false
	}
	v, err := domain.ParseVersion(ver)
	if err != nil {
		return false
	}
	switch v.Major {
	case 8, 11, 17, 21:
		return true
	}
	return false
}

// vendorHint 对 java 提示可指定的 --vendor 选项。
func vendorHint(candID, vendor string) string {
	if candID == "java" && vendor == "" {
		return "  (--vendor tem|zul|graal|kona|dragonwell|sap：省略则显示所有发行版)"
	}
	return ""
}

func newListCmd() *cobra.Command {
	var vendor string
	cmd := &cobra.Command{
		Use:   "list [candidate]",
		Short: "列出可安装的远程版本",
		Long: `列出远程可安装的 SDK 版本（首次需联网拉取元数据，之后使用本地缓存）。

参数 candidate 可省略：省略时列出全部 SDK 的可用版本；指定时只列该 SDK。
  支持的 candidate: java, go, node, maven, gradle

输出说明:
  结果按紧凑列表展示，每行一个版本标识（即安装时使用的版本号）。
    标识示例：21.0.12.1+1-tem、21.0.12.1-zul、25.0.4.1-graal、1.23.4、22.12.0 等。
    行尾标记 * 表示当前使用、> 表示本机已安装；lts 表示 LTS 版本。
  结果超过一屏时进入交互式翻页（空格/↓ 下一页，b/↑ 上一页，q 退出）。
  java 有多个发行版（厂商）：省略 --vendor 时默认列出全部发行版版本；指定时只列该
    发行版（tem=Eclipse Temurin，zul=Azul Zulu，graal=GraalVM Community）。

示例:
  sdkz list                  # 列出全部 SDK 的可安装版本
  sdkz list java             # 列出 Java 全部发行版版本（默认行为）
  sdkz list java --vendor zul   # 只看 Azul Zulu 的 Java 版本
  sdkz list go               # 列出 Go 版本`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				return renderList(cmd, m, args[0], vendor)
			}
			for _, c := range m.Candidates() {
				if err := renderList(cmd, m, c.ID, vendor); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&vendor, "vendor", "", "按发行版过滤")
	return cmd
}

func renderList(cmd *cobra.Command, m *service.Manager, candID, vendor string) error {
	cand, err := m.FindCandidate(candID)
	if err != nil {
		return err
	}
	rels, err := m.ListRemote(cmd.Context(), candID, vendor)
	if err != nil {
		return fmt.Errorf("获取 %s 版本清单失败: %w", candID, err)
	}
	installed, _ := m.ListInstalled(candID)
	current, _ := m.Current(candID)
	if len(rels) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n  （无可用版本：请检查网络后重试）\n",
			cand.Name)
		return nil
	}
	installedSet := map[string]bool{}
	for _, v := range installed {
		installedSet[v] = true
	}
	relLTSMap := map[string]bool{}
	for _, r := range rels {
		relLTSMap[r.Version] = r.LTS
	}

	// 按 vendor 分组、组内按版本降序；vendorOrder 维持候选定义顺序。
	byVendor := map[string][]string{}
	var vendorOrder []string
	for _, r := range rels {
		if _, ok := byVendor[r.VendorID]; !ok {
			vendorOrder = append(vendorOrder, r.VendorID)
		}
		byVendor[r.VendorID] = append(byVendor[r.VendorID], r.Version)
	}
	for _, vs := range byVendor {
		domain.SortVersionsDesc(vs)
	}

	color := isTTY(asFile(cmd.OutOrStdout()))
	plat := platform.Detect()
	var lines []string
	lines = append(lines, cand.Name)
	lines = append(lines, fmt.Sprintf("  平台: %s/%s   共 %d 个版本", plat.OS, plat.Arch, len(rels)))
	if vendor != "" {
		lines = append(lines, fmt.Sprintf("  发行版过滤: %s", vendor))
	}
	multiVendor := len(vendorOrder) > 1

	// 紧凑列表：每行 = 缩进 + 版本标识 + 对齐后的状态标记。
	// 多厂商时按厂商分组，组头显示厂商名；单厂商直接列版本。
	// 这种格式不依赖终端宽度，避免在窄终端里表格折行后错位。
	if vendor == "" && len(cand.Vendors) > len(byVendor) {
		var missing []string
		for _, v := range cand.Vendors {
			if _, ok := byVendor[v.ID]; !ok {
				missing = append(missing, v.Name)
			}
		}
		if len(missing) > 0 {
			lines = append(lines, "")
			lines = append(lines, colorStr(color, cYellow, fmt.Sprintf("  以下发行版未获取到版本（可能网络受限）: %s", strings.Join(missing, ", "))))
		}
	}

	for _, vid := range vendorOrder {
		v, _ := cand.FindVendor(vid)
		vendorName := vid
		if v != nil {
			vendorName = v.Name
		}
		if multiVendor {
			lines = append(lines, "  "+colorStr(color, cBold, vendorName))
		}

		// 计算本组最长版本标识的显示宽度，用于状态标记对齐。
		maxW := 0
		for _, ver := range byVendor[vid] {
			if w := displayWidth(ver); w > maxW {
				maxW = w
			}
		}

		for _, ver := range byVendor[vid] {
			verStr := ver
			lts := isLTS(candID, ver) // java 按主版本判断
			switch {
			case ver == current:
				verStr = colorStr(color, cGreen, ver)
			case installedSet[ver]:
				verStr = colorStr(color, cCyan, ver)
			}
			var marks []string
			if ver == current {
				marks = append(marks, colorStr(color, cGreen, "*"))
			}
			if installedSet[ver] {
				marks = append(marks, colorStr(color, cCyan, ">"))
			}
			// Node 等用源里的 LTS 标记（rels 中带 LTS 字段）。
			if relLTS, ok := relLTSMap[ver]; ok && relLTS {
				lts = true
			}
			if lts {
				marks = append(marks, colorStr(color, cYellow, "lts"))
			}
			markStr := strings.Join(marks, " ")
			pad := maxW - displayWidth(verStr)
			line := fmt.Sprintf("    %s%s%s", verStr, strings.Repeat(" ", pad+2), markStr)
			lines = append(lines, line)
		}
	}
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("安装: sdkz install %s <标识>%s", candID, vendorHint(candID, vendor)))
	lines = append(lines, fmt.Sprintf("设为默认: sdkz default %s <标识>", candID))
	lines = append(lines, "图例: * 当前使用   > 已安装   lts LTS版本")
	outputLines(cmd.OutOrStdout(), lines)
	return nil
}

// stripANSI 移除字符串中的 ANSI 转义序列。
var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// displayWidth 返回字符串在终端上的显示宽度（按 rune 计，不含 ANSI 转义）。
func displayWidth(s string) int { return utf8.RuneCountInString(stripANSI(s)) }

// truncate 截断字符串到 max 宽度（按 rune 计），超出加 …
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

func newLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls [candidate]",
		Short: "列出本地已安装的版本",
		Long: `列出本地已经安装到数据目录的 SDK 版本。

参数 candidate 可省略：省略时列出全部已安装 SDK；指定时只列该 SDK。
  支持的 candidate: java, go, node, maven, gradle

标记说明:
  *  当前使用（current 指向版本，影响新终端默认使用的版本）
  >  已安装

示例:
  sdkz ls           # 列出所有已安装的 SDK 版本
  sdkz ls java      # 只列出已安装的 Java 版本`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manager()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				return renderInstalled(cmd, m, args[0])
			}
			for _, c := range m.Candidates() {
				if err := renderInstalled(cmd, m, c.ID); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func renderInstalled(cmd *cobra.Command, m *service.Manager, candID string) error {
	cand, err := m.FindCandidate(candID)
	if err != nil {
		return err
	}
	installed, err := m.ListInstalled(candID)
	if err != nil {
		return err
	}
	current, _ := m.Current(candID)
	domain.SortVersionsDesc(installed)

	color := isTTY(asFile(cmd.OutOrStdout()))
	var lines []string
	lines = append(lines, cand.Name)
	if len(installed) == 0 {
		lines = append(lines, fmt.Sprintf("  （无已安装版本，使用 'sdkz install %s <版本>' 安装）", candID))
		lines = append(lines, "")
		outputLines(cmd.OutOrStdout(), lines)
		return nil
	}

	maxW := 0
	for _, ver := range installed {
		if w := displayWidth(ver); w > maxW {
			maxW = w
		}
	}
	for _, ver := range installed {
		verStr := ver
		if ver == current {
			verStr = colorStr(color, cGreen, ver)
		}
		mark := ""
		if ver == current {
			mark = colorStr(color, cGreen, "*")
		}
		pad := maxW - displayWidth(verStr)
		lines = append(lines, fmt.Sprintf("  %s%s%s", verStr, strings.Repeat(" ", pad+2), mark))
	}
	lines = append(lines, "")
	lines = append(lines, "图例: * 当前使用   设为默认: sdkz default "+candID+" <版本>")
	outputLines(cmd.OutOrStdout(), lines)
	return nil
}
