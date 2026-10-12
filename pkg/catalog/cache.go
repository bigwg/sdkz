package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sdkz/pkg/domain"
)

// MetadataTTL 为版本清单缓存的新鲜期。
// 取 1 小时的依据：GitHub 匿名 API 限额为 60 次/小时/IP（按整点窗口重置），
// 一次 list java 约发 4 个 GitHub 请求；TTL=1h 时 GitHub 请求量被压到
// 每小时个位数，配额占用不足 15%，同时 JDK/Node 等发版节奏为天级，
// 1 小时的数据延迟对选型无实际影响。过期后先尝试网络，失败再回退旧缓存。
const MetadataTTL = time.Hour

type cacheFile struct {
	Generated time.Time         `json:"generated"`
	Platform  string            `json:"platform"`
	Releases  []*domain.Release `json:"releases"`
}

// CachePath 返回某候选+发行版的缓存文件路径（~/.sdkz/metadata/ 下）。
func (m *Manager) CachePath(candID, vendorID string) string {
	return filepath.Join(m.cfg.Root, "metadata", fmt.Sprintf("candidates-%s-%s.json", candID, vendorID))
}

// readCache 读取本地缓存；平台不匹配或文件损坏视为未命中。
func (m *Manager) readCache(candID, vendorID string) ([]*domain.Release, time.Time, bool) {
	data, err := os.ReadFile(m.CachePath(candID, vendorID))
	if err != nil {
		return nil, time.Time{}, false
	}
	var cf cacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, time.Time{}, false
	}
	if cf.Platform != m.Platform().String() || len(cf.Releases) == 0 {
		return nil, time.Time{}, false
	}
	return cf.Releases, cf.Generated, true
}

// writeCache 原子写入缓存（先写临时文件再重命名，避免读到半截 JSON）。
// 空列表拒绝写入：空缓存无法用于回退，只会污染可用性判断。
func (m *Manager) writeCache(candID, vendorID string, rels []*domain.Release) {
	if len(rels) == 0 {
		return
	}
	path := m.CachePath(candID, vendorID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	cf := cacheFile{Generated: time.Now(), Platform: m.Platform().String(), Releases: rels}
	data, err := json.Marshal(cf)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
