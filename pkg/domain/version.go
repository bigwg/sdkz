package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version 是从发行版字符串中提取的数值版本号。
// 容忍多种前缀形式：21.0.5+11、go1.23.4、v22.11.0、8.10.2-rc-1。
type Version struct {
	Major      int
	Minor      int
	Patch      int
	HasMinor   bool
	HasPatch   bool
	PreRelease bool // 包含 rc/beta/ea/alpha/preview 等
	Raw        string
}

var (
	numGroupRe   = regexp.MustCompile(`(\d+)(?:\.(\d+))?(?:\.(\d+))?`)
	preReleaseRe = regexp.MustCompile(`(?i)(rc|beta|ea|alpha|preview|milestone|m\d)`)
)

// ParseVersion 从任意发行版字符串中提取版本号。
func ParseVersion(s string) (Version, error) {
	m := numGroupRe.FindStringSubmatch(s)
	if m == nil || m[1] == "" {
		return Version{}, fmt.Errorf("无法从 %q 中解析版本号", s)
	}
	v := Version{Raw: s, PreRelease: preReleaseRe.MatchString(s)}
	var err error
	if v.Major, err = strconv.Atoi(m[1]); err != nil {
		return Version{}, err
	}
	if m[2] != "" {
		v.HasMinor = true
		if v.Minor, err = strconv.Atoi(m[2]); err != nil {
			return Version{}, err
		}
	}
	if m[3] != "" {
		v.HasPatch = true
		if v.Patch, err = strconv.Atoi(m[3]); err != nil {
			return Version{}, err
		}
	}
	return v, nil
}

// MustParse 解析失败时返回零值（用于排序等宽松场景）。
func MustParse(s string) Version {
	v, _ := ParseVersion(s)
	return v
}

// Compare 比较两版本：-1 / 0 / 1。
func (v Version) Compare(o Version) int {
	compareNum := func(a, b int) int {
		switch {
		case a < b:
			return -1
		case a > b:
			return 1
		}
		return 0
	}
	if c := compareNum(v.Major, o.Major); c != 0 {
		return c
	}
	if c := compareNum(v.Minor, o.Minor); c != 0 {
		return c
	}
	return compareNum(v.Patch, o.Patch)
}

// MatchVersion 在 versions 中匹配版本规格，返回最优原始版本字符串。
//
//	spec 支持：
//	  "" / "latest"            → 最高 GA 版本
//	  "21"                     → major=21 的最高版本
//	  "21.0"                   → major=21 minor=0 的最高版本
//	  "21.0.5"                 → 精确（GA 优先，无则允许 pre-release）
func MatchVersion(spec string, versions []string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		spec = "latest"
	}
	sp := MustParse(spec)
	isLatest := spec == "latest"

	bestGA := ""
	bestPre := ""
	// 全量扫描，分别记录最优 GA 与最优 pre。
	for _, s := range versions {
		v := MustParse(s)
		if !isLatest && !numMatches(v, sp) {
			continue
		}
		ga := !v.PreRelease
		best := &bestGA
		if !ga {
			best = &bestPre
		}
		if *best == "" {
			*best = s
			continue
		}
		// 相同数值段时选择"更完整"的版本（如 21.0.5 优于 21.0.5+11 之外的补丁粒度）：
		// 简单起见按数值比较，相等时保留原值。
		cur := MustParse(*best)
		if v.Compare(cur) > 0 {
			*best = s
		}
	}
	if bestGA != "" {
		return bestGA, nil
	}
	if bestPre != "" {
		return bestPre, nil
	}
	return "", fmt.Errorf("没有找到匹配版本 %q 的发行版", spec)
}

// numMatches 判断 v 是否满足 spec 的数值约束。
func numMatches(v, spec Version) bool {
	if v.Major != spec.Major {
		return false
	}
	if spec.HasMinor && (!v.HasMinor || v.Minor != spec.Minor) {
		return false
	}
	if spec.HasPatch && (!v.HasPatch || v.Patch != spec.Patch) {
		return false
	}
	return true
}

// IsPreReleaseString 判断原始字符串是否为预发布版本。
func IsPreReleaseString(s string) bool {
	return preReleaseRe.MatchString(s)
}

// SortVersionsDesc 按数值降序排序原始版本字符串（GA 优先于 pre-release）。
func SortVersionsDesc(versions []string) {
	for i := 0; i < len(versions); i++ {
		for j := i + 1; j < len(versions); j++ {
			vi, vj := MustParse(versions[i]), MustParse(versions[j])
			// vj 优于 vi 时交换（选择排序，i 位最终为剩余最优）。
			better := vj.Compare(vi) > 0 || (vj.Compare(vi) == 0 && !vj.PreRelease && vi.PreRelease)
			if better {
				versions[i], versions[j] = versions[j], versions[i]
			}
		}
	}
}

// HighestGA 返回版本列表中数值最高的 GA 版本（无 GA 时返回最高 pre-release）。
func HighestGA(versions []string) (string, error) {
	if len(versions) == 0 {
		return "", errors.New("版本列表为空")
	}
	best := ""
	for _, s := range versions {
		if best == "" {
			best = s
			continue
		}
		cur, cand := MustParse(best), MustParse(s)
		if cand.PreRelease && !cur.PreRelease {
			continue
		}
		if cand.Compare(cur) > 0 {
			best = s
		}
	}
	return best, nil
}
