package domain

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in      string
		major   int
		minor   int
		patch   int
		pre     bool
		wantErr bool
	}{
		{"21.0.5+11", 21, 0, 5, false, false},
		{"go1.23.4", 1, 23, 4, false, false},
		{"v22.11.0", 22, 11, 0, false, false},
		{"8.10.2-rc-1", 8, 10, 2, true, false},
		{"21.0.5-ea", 21, 0, 5, true, false},
		{"no-version-here", 0, 0, 0, false, true},
	}
	for _, c := range cases {
		v, err := ParseVersion(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: 期望错误", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if v.Major != c.major || v.Minor != c.minor || v.Patch != c.patch {
			t.Errorf("%s: 解析为 %d.%d.%d，期望 %d.%d.%d",
				c.in, v.Major, v.Minor, v.Patch, c.major, c.minor, c.patch)
		}
		if v.PreRelease != c.pre {
			t.Errorf("%s: pre=%v，期望 %v", c.in, v.PreRelease, c.pre)
		}
	}
}

func TestMatchVersion(t *testing.T) {
	versions := []string{
		"21.0.5-tem", "21.0.2-tem", "17.0.10-tem", "11.0.24-tem",
		"21.0.5-ea-tem", "23.0.1-tem", "8.0.432-tem",
	}
	cases := []struct {
		spec string
		want string
	}{
		{"", "23.0.1-tem"},
		{"latest", "23.0.1-tem"},
		{"21", "21.0.5-tem"}, // GA 优先于 ea
		{"17", "17.0.10-tem"},
		{"21.0", "21.0.5-tem"},
		{"21.0.5", "21.0.5-tem"},
		{"11.0", "11.0.24-tem"},
	}
	for _, c := range cases {
		got, err := MatchVersion(c.spec, versions)
		if err != nil {
			t.Errorf("MatchVersion(%q): %v", c.spec, err)
			continue
		}
		if got != c.want {
			t.Errorf("MatchVersion(%q) = %q，期望 %q", c.spec, got, c.want)
		}
	}
	if _, err := MatchVersion("99", versions); err == nil {
		t.Error("MatchVersion(99) 应失败")
	}
}

func TestSortVersionsDesc(t *testing.T) {
	in := []string{"21.0.2-tem", "17.0.10-tem", "23.0.1-tem", "21.0.5-tem"}
	SortVersionsDesc(in)
	if in[0] != "23.0.1-tem" || in[1] != "21.0.5-tem" || in[2] != "21.0.2-tem" || in[3] != "17.0.10-tem" {
		t.Errorf("排序结果错误: %v", in)
	}
}

func TestHighestGA(t *testing.T) {
	versions := []string{"21.0.5-ea", "21.0.2", "23.0.1"}
	got, err := HighestGA(versions)
	if err != nil {
		t.Fatal(err)
	}
	if got != "23.0.1" {
		t.Errorf("HighestGA = %q，期望 23.0.1", got)
	}
}
