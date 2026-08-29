package sources

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"sdkz/pkg/domain"
	"sdkz/pkg/platform"
)

const ghDefaultBase = "https://api.github.com"

// ghReleaseSource 基于 GitHub Releases 的通用 JDK 发行版源。
// 通过配置支持命名规律各异的厂商（Tencent Kona / Alibaba Dragonwell / SAP Machine 等）。
type ghReleaseSource struct {
	base
	client  *http.Client
	id      string
	name    string
	repos   []string // 形如 /repos/<owner>/<repo>/releases?per_page=100
	suffix  string   // 版本标识后缀，如 -kona / -dragonwell / -sap
	matcher func(name string, p platform.Platform) (string, bool)
}

func newGHReleaseSource(id, name string, client *http.Client, repos []string, suffix string, matcher func(string, platform.Platform) (string, bool)) *ghReleaseSource {
	return &ghReleaseSource{
		base:    base{base: ghDefaultBase},
		client:  client,
		id:      id,
		name:    name,
		repos:   repos,
		suffix:  suffix,
		matcher: matcher,
	}
}

func (s *ghReleaseSource) ID() string   { return s.id }
func (s *ghReleaseSource) Name() string { return s.name }

func (s *ghReleaseSource) Fetch(ctx context.Context, p platform.Platform) ([]*domain.Release, error) {
	var rels []*domain.Release
	seen := map[string]bool{}
	failCount := 0
	var lastErr error
	for _, repo := range s.repos {
		var releases []ghRelease
		url := s.join(repo)
		if err := getJSON(ctx, s.client, url, &releases); err != nil {
			// 单个主版本仓库不可用（如尚未发布、被限流）时记录并继续，不拖垮整个厂商。
			failCount++
			lastErr = err
			continue
		}
		for _, r := range releases {
			for _, a := range r.Assets {
				ver, ok := s.matcher(a.Name, p)
				if !ok || ver == "" {
					continue
				}
				id := ver + s.suffix
				if seen[id] {
					continue
				}
				seen[id] = true
				ext := "tar.gz"
				if strings.HasSuffix(a.Name, ".zip") {
					ext = "zip"
				}
				rels = append(rels, &domain.Release{
					Version: id,
					Stable:  true,
					Artifact: &domain.Artifact{
						URL:          a.URL,
						Ext:          ext,
						Strip:        1,
						ChecksumType: "sha256",
					},
				})
				break
			}
		}
	}
	// 所有仓库都拉取失败（如 GitHub 限流、网络中断）时返回错误，交由上层告警/回退缓存。
	if failCount == len(s.repos) && len(rels) == 0 && lastErr != nil {
		return nil, fmt.Errorf("获取 %s 版本清单失败: %w", s.name, lastErr)
	}
	return rels, nil
}

// ---- 平台/资产名匹配辅助 ----

// ghOSMap / ghArchMap 将本程序平台映射到 GitHub 资产命名惯例。
var (
	ghOSMap   = map[string]string{"linux": "linux", "darwin": "macos", "windows": "windows"}
	ghArchMap = map[string]string{"amd64": "x64", "arm64": "aarch64"}
)

// matchPlatform 检查资产名中是否包含 <os>_<arch> 或 <os>-<arch> 平台段（兼容多种分隔）。
func matchPlatform(name string, p platform.Platform) bool {
	targetOS := ghOSMap[p.OS]
	targetArch := ghArchMap[p.Arch]
	if targetOS == "" || targetArch == "" {
		return false
	}
	n := strings.ToLower(name)
	// 常见形式：linux-x64 / linux_x64 / x64_linux / aarch64_linux
	candidates := []string{
		targetOS + "-" + targetArch,
		targetOS + "_" + targetArch,
		targetArch + "_" + targetOS,
		targetArch + "-" + targetOS,
	}
	for _, c := range candidates {
		if strings.Contains(n, c) {
			return true
		}
	}
	return false
}

// ---- 各厂商版本提取 ----

// parseKona 解析 Tencent Kona 资产名，兼容多种命名：
//   TencentKona-21.0.12.b1-jdk_linux-aarch64.tar.gz -> 21.0.12
//   TencentKona-11.0.32.b1-jdk_linux-x64.tar.gz     -> 11.0.32
//   TencentKona8.0.27.b1_qdk_linux-aarch64_8u502.tar.gz -> 8.0.27
func parseKona(name string, p platform.Platform) (string, bool) {
	if !matchPlatform(name, p) {
		return "", false
	}
	if !strings.Contains(name, "TencentKona") {
		return "", false
	}
	// 取 TencentKona 之后、首个 jdk/qdk 段之前的部分。
	rest := strings.TrimPrefix(name, "TencentKona")
	rest = strings.TrimPrefix(rest, "-")
	// 在 -jdk / _qdk / -qdk 之前截断。
	for _, sep := range []string{"-jdk", "_qdk", "-qdk", "_jdk"} {
		if idx := strings.Index(rest, sep); idx >= 0 {
			rest = rest[:idx]
			break
		}
	}
	if rest == "" {
		return "", false
	}
	// 去掉末尾的 build 号段（如 .b1），仅当该段含字母时。
	if dot := strings.LastIndex(rest, "."); dot > 0 {
		tail := rest[dot+1:]
		if regexp.MustCompile(`[a-zA-Z]`).MatchString(tail) {
			rest = rest[:dot]
		}
	}
	return rest, true
}

// parseDragonwell 解析 Alibaba Dragonwell 资产名：
//   Alibaba_Dragonwell_Extended_21.0.11.0.11.10_x64_linux.tar.gz -> 21.0.11.0.11.10
//   Alibaba_Dragonwell_21.0.12.0.8_x64_linux.tar.gz             -> 21.0.12.0.8
func parseDragonwell(name string, p platform.Platform) (string, bool) {
	if !strings.Contains(name, "Dragonwell") || !matchPlatform(name, p) {
		return "", false
	}
	if !strings.HasSuffix(name, ".tar.gz") {
		return "", false
	}
	// 取最后一个 _<arch>_ 之前的版本段
	parts := strings.Split(strings.TrimSuffix(name, ".tar.gz"), "_")
	// 找到 x64/aarch64 的位置，其前一段为版本
	for i, part := range parts {
		if part == ghArchMap[p.Arch] && i > 0 {
			return parts[i-1], true
		}
	}
	return "", false
}

// parseSAP 解析 SAP Machine 资产名：
//   sapmachine-jdk-27-ea.35_linux-x64_bin.tar.gz -> 27-ea.35
//   sapmachine-jdk-21.0.12_linux-aarch64_bin.tar.gz -> 21.0.12
func parseSAP(name string, p platform.Platform) (string, bool) {
	if !strings.HasPrefix(name, "sapmachine-jdk-") || !matchPlatform(name, p) {
		return "", false
	}
	if !strings.Contains(name, "_bin.tar.gz") {
		return "", false
	}
	rest := strings.TrimPrefix(name, "sapmachine-jdk-")
	// 27-ea.35_linux-x64_bin -> 取 _ 之前
	if idx := strings.Index(rest, "_"); idx >= 0 {
		rest = rest[:idx]
	}
	return rest, true
}

// ---- 各厂商源构造 ----

// NewKonaSource 构造腾讯 Kona 源。
// 仓库按主版本划分：Tencent/TencentKona-{8,11,17,21}。
func NewKonaSource(client *http.Client) *ghReleaseSource {
	repos := []string{}
	for _, maj := range []string{"8", "11", "17", "21"} {
		repos = append(repos, "/repos/Tencent/TencentKona-"+maj+"/releases?per_page=100")
	}
	return newGHReleaseSource("kona", "Tencent Kona", client, repos, "-kona", parseKona)
}

// NewDragonwellSource 构造阿里 Dragonwell 源。
// 仓库按主版本划分：dragonwell-project/dragonwell{8,11,17,21}。
func NewDragonwellSource(client *http.Client) *ghReleaseSource {
	repos := []string{}
	for _, maj := range []string{"8", "11", "17", "21"} {
		repos = append(repos, "/repos/dragonwell-project/dragonwell"+maj+"/releases?per_page=100")
	}
	return newGHReleaseSource("dragonwell", "Alibaba Dragonwell", client, repos, "-dragonwell", parseDragonwell)
}

// NewSAPMachineSource 构造 SAP Machine 源。
// 单一仓库 SAP/SapMachine 包含所有主版本。
func NewSAPMachineSource(client *http.Client) *ghReleaseSource {
	return newGHReleaseSource("sap", "SAP Machine", client,
		[]string{"/repos/SAP/SapMachine/releases?per_page=100"}, "-sap", parseSAP)
}
