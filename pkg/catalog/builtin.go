package catalog

import "sdkz/pkg/domain"

// builtinCandidates 返回 v1 内置候选定义。
func builtinCandidates() []*domain.Candidate {
	return []*domain.Candidate{
		{
			ID:            "java",
			Name:          "Java (JDK)",
			HomeEnv:       "JAVA_HOME",
			BinDir:        "bin",
			Default:       "21",
			HasVendors:    true,
			DefaultVendor: "tem",
			Vendors: []domain.Vendor{
				{ID: "tem", Name: "Eclipse Temurin", SourceID: "temurin"},
				{ID: "zul", Name: "Azul Zulu", SourceID: "zulu"},
				{ID: "graal", Name: "GraalVM Community", SourceID: "graalvm"},
				{ID: "kona", Name: "Tencent Kona", SourceID: "kona"},
				{ID: "dragonwell", Name: "Alibaba Dragonwell", SourceID: "dragonwell"},
				{ID: "sap", Name: "SAP Machine", SourceID: "sap"},
			},
		},
		{
			ID:      "go",
			Name:    "Go",
			HomeEnv: "GOROOT",
			BinDir:  "bin",
			Default: "latest",
			Vendors: []domain.Vendor{{ID: "official", Name: "Go Official", SourceID: "golang"}},
		},
		{
			ID:      "node",
			Name:    "Node.js",
			BinDir:  "bin",
			Default: "lts",
			Vendors: []domain.Vendor{{ID: "official", Name: "Node.js Official", SourceID: "nodejs"}},
		},
		{
			ID:      "maven",
			Name:    "Apache Maven",
			HomeEnv: "MAVEN_HOME",
			BinDir:  "bin",
			Default: "latest",
			Vendors: []domain.Vendor{{ID: "official", Name: "Apache Maven", SourceID: "maven"}},
		},
		{
			ID:      "gradle",
			Name:    "Gradle",
			HomeEnv: "GRADLE_HOME",
			BinDir:  "bin",
			Default: "latest",
			Vendors: []domain.Vendor{{ID: "official", Name: "Gradle", SourceID: "gradle"}},
		},
	}
}
