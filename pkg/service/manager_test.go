package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"sdkz/pkg/platform"
)

// newMultiVendorFixture 构造多发行版假源：tem/graal/dragonwell 各返回一个 21 系版本，
// 其余源与仓库路径返回空清单。用于验证跨发行版的版本匹配行为（不触网）。
func newMultiVendorFixture(t *testing.T) *httptest.Server {
	t.Helper()
	p := platform.Detect()
	ghOS := map[string]string{"linux": "linux", "darwin": "macos", "windows": "windows"}[p.OS]
	ghArch := map[string]string{"amd64": "x64", "arm64": "aarch64"}[p.Arch]
	graalAsset := fmt.Sprintf("graalvm-community-jdk-21.0.2_%s-%s_bin.tar.gz", ghOS, ghArch)
	dragonwellExt := ".tar.gz"
	if p.OS == "windows" {
		dragonwellExt = ".zip"
	}
	dragonwellAsset := fmt.Sprintf("Alibaba_Dragonwell_21.0.2.0.2.13_%s_%s%s", ghArch, ghOS, dragonwellExt)
	graalJSON := fmt.Sprintf(`[{"tag_name":"vm-21.0.2","assets":[{"name":%q,"browser_download_url":"https://example.com/graal21.tar.gz"}]}]`, graalAsset)
	dragonwellJSON := fmt.Sprintf(`[{"tag_name":"v21.0.2.0.2.13","assets":[{"name":%q,"browser_download_url":"https://example.com/dw21.tar.gz"}]}]`, dragonwellAsset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/graalvm/graalvm-ce-builds/releases":
			fmt.Fprint(w, graalJSON)
		case "/repos/dragonwell-project/dragonwell21/releases":
			fmt.Fprint(w, dragonwellJSON)
		case "/v3/info/available_releases":
			fmt.Fprint(w, `{"available_releases":[21]}`)
		case "/v3/assets/latest/21/hotspot":
			fmt.Fprint(w, `[{"version":{"openjdk_version":"21.0.5+11-TS"},"binary":{"package":{
				"name":"jdk21.tar.gz","link":"https://example.com/jdk21.tar.gz"}}}]`)
		default:
			fmt.Fprint(w, `[]`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMatchReleasesExactSpecShortCircuit(t *testing.T) {
	srv := newMultiVendorFixture(t)
	m, err := NewWithRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"temurin", "zulu", "graalvm", "kona", "dragonwell", "sap"} {
		m.SetSourceBaseURL(id, srv.URL)
	}
	ctx := context.Background()
	cand, err := m.FindCandidate("java")
	if err != nil {
		t.Fatal(err)
	}

	// 1. 完整版本串精确匹配：直接返回唯一结果，跳过多发行版选择。
	rels, err := m.MatchReleases(ctx, cand, "21.0.2-graal", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 || rels[0].Version != "21.0.2-graal" || rels[0].VendorID != "graal" {
		t.Fatalf("精确版本应短路返回唯一 graal 结果，得到 %+v", rels)
	}

	// 2. Temurin 完整版本串（含构建号+发行版后缀）同样短路。
	rels, err = m.MatchReleases(ctx, cand, "21.0.5+11-tem", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 1 || rels[0].VendorID != "tem" {
		t.Fatalf("精确版本应短路返回唯一 tem 结果，得到 %+v", rels)
	}

	// 3. 模糊规格（如 21）跨发行版仍有多个候选，保持交互选择行为。
	rels, err = m.MatchReleases(ctx, cand, "21", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) < 2 {
		t.Fatalf("模糊规格应返回多发行版候选，得到 %+v", rels)
	}
}
