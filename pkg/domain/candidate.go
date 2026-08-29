// Package domain 定义 sdkz 的核心领域模型。
package domain

// Candidate 表示一种可管理的 SDK 候选（如 Java、Go）。
type Candidate struct {
	ID            string   // java / go / node / maven / gradle
	Name          string   // 展示名，如 "Java (JDK)"
	HomeEnv       string   // 导出的 HOME 环境变量名，如 JAVA_HOME；空表示无
	BinDir        string   // 相对 bin 目录，通常 "bin"
	Default       string   // 默认安装版本规格："21" / "lts" / "latest"
	HasVendors    bool     // 是否存在多个发行版（vendor）
	DefaultVendor string   // 默认 vendor id（HasVendors 为 true 时生效）
	Vendors       []Vendor // 发行版列表
}

// Vendor 表示某一候选下的发行版（如 Java 的 Temurin）。
type Vendor struct {
	ID       string // tem / zul / graal / official
	Name     string // 展示名
	SourceID string // 对应 catalog sources 适配器 id
}

// FindVendor 按 id 查找发行版。
func (c *Candidate) FindVendor(id string) (*Vendor, bool) {
	if c.HasVendors {
		for i := range c.Vendors {
			if c.Vendors[i].ID == id {
				return &c.Vendors[i], true
			}
		}
	}
	return nil, false
}

// VendorIDs 返回所有发行版 id（无 vendor 时返回空切片）。
func (c *Candidate) VendorIDs() []string {
	ids := make([]string, 0, len(c.Vendors))
	for _, v := range c.Vendors {
		ids = append(ids, v.ID)
	}
	return ids
}
