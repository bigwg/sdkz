// Package installer 负责 SDK 的安装/卸载事务。
package installer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sdkz/pkg/config"
	"sdkz/pkg/domain"
	"sdkz/pkg/download"
	"sdkz/pkg/platform"
)

// ProgressFunc 安装进度回调。
type ProgressFunc = domain.ProgressFunc

// ErrAlreadyInstalled 版本已安装。
var ErrAlreadyInstalled = errors.New("已安装")

// ErrIsCurrent 版本是当前使用版本。
var ErrIsCurrent = errors.New("该版本为当前使用版本")

// Installer 管理安装与卸载。
type Installer struct {
	cfg    *config.Config
	client *http.Client
	pl     platform.Platform
	// Warn 用于非致命告警。
	Warn func(format string, args ...any)
}

// NewInstaller 构造安装器。
func NewInstaller(cfg *config.Config) *Installer {
	return &Installer{
		cfg:    cfg,
		client: &http.Client{Timeout: 5 * time.Minute},
		pl:     platform.Detect(),
		Warn:   func(string, ...any) {},
	}
}

// CandidateDir 返回候选根目录。
func (i *Installer) CandidateDir(candID string) string {
	return filepath.Join(i.cfg.Root, "candidates", candID)
}

// VersionDir 返回版本目录。
func (i *Installer) VersionDir(candID, ver string) string {
	return filepath.Join(i.CandidateDir(candID), ver)
}

// IsInstalled 判断版本是否已安装。
func (i *Installer) IsInstalled(candID, ver string) bool {
	fi, err := os.Stat(i.VersionDir(candID, ver))
	return err == nil && fi.IsDir()
}

// Installed 返回已安装的版本目录名（排除 current / 隐藏项）。
func (i *Installer) Installed(candID string) ([]string, error) {
	entries, err := os.ReadDir(i.CandidateDir(candID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() {
			continue
		}
		if name == "current" || strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// Install 执行完整安装事务。返回安装的版本目录名。
func (i *Installer) Install(ctx context.Context, rel *domain.Release, progress ProgressFunc) (string, error) {
	if rel == nil || rel.Artifact == nil {
		return "", errors.New("发行版缺少产物信息")
	}
	candDir := i.CandidateDir(rel.CandidateID)
	verDir := i.VersionDir(rel.CandidateID, rel.Version)
	if _, err := os.Stat(verDir); err == nil {
		return rel.Version, fmt.Errorf("%s %s %w", rel.CandidateID, rel.Version, ErrAlreadyInstalled)
	}
	if err := os.MkdirAll(candDir, 0o755); err != nil {
		return "", err
	}
	tmpDir := filepath.Join(i.cfg.Root, "tmp", rel.CandidateID)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", err
	}

	art := rel.Artifact
	ext := art.ArchiveExt()
	archive := filepath.Join(tmpDir, rel.Version+"."+ext)

	// 1. 下载（主地址 + fallback）。
	urls := append([]string{i.cfg.ApplyMirror(art.URL)}, applyMirrors(i.cfg, art.FallbackURLs)...)
	if err := i.downloadWithFallback(ctx, urls, archive, progress); err != nil {
		return "", err
	}
	if progress != nil {
		progress(0, 0, "校验")
	}

	// 2. 校验。
	checksum := ""
	switch {
	case art.SHA256 != "":
		checksum = art.SHA256
	case art.ChecksumURL != "":
		sumURL := i.cfg.ApplyMirror(art.ChecksumURL)
		var err error
		checksum, err = FetchChecksum(ctx, i.client, sumURL, art.ChecksumType)
		if err != nil {
			i.Warn("获取校验值失败（%v），跳过校验", err)
			checksum = ""
		}
	default:
		i.Warn("该版本无校验值，跳过哈希校验")
	}
	if checksum != "" {
		if err := VerifyFile(archive, checksum, art.ChecksumType); err != nil {
			os.Remove(archive)
			return "", err
		}
	}
	if progress != nil {
		progress(0, 0, "解压")
	}

	// 3. 解压到临时目录。
	staging := filepath.Join(tmpDir, rel.Version+".d")
	os.RemoveAll(staging)
	if err := Extract(ctx, archive, staging, art.Strip); err != nil {
		os.Remove(archive)
		return "", fmt.Errorf("解压失败: %w", err)
	}
	os.Remove(archive)

	// 4. 原子落盘。
	if err := os.Rename(staging, verDir); err != nil {
		// 目标已存在（并发）→ 清理后重试。
		if _, statErr := os.Stat(verDir); statErr == nil {
			os.RemoveAll(verDir)
			if err2 := os.Rename(staging, verDir); err2 != nil {
				return "", err2
			}
		} else {
			return "", err
		}
	}
	return rel.Version, nil
}

func (i *Installer) downloadWithFallback(ctx context.Context, urls []string, dest string, progress ProgressFunc) error {
	var lastErr error
	for _, u := range urls {
		err := download.Download(ctx, i.client, u, dest, func(done, total int64) {
			if progress != nil {
				progress(done, total, "下载")
			}
		})
		if err == nil {
			return nil
		}
		lastErr = err
		os.Remove(dest + ".part")
		i.Warn("下载 %s 失败: %v", u, err)
	}
	return lastErr
}

// Uninstall 卸载版本目录。
func (i *Installer) Uninstall(candID, ver string) error {
	verDir := i.VersionDir(candID, ver)
	if _, err := os.Stat(verDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s %s 未安装", candID, ver)
		}
		return err
	}
	// Windows 上删除被占用目录可能失败：先改名，失败则登记待清理。
	if err := os.RemoveAll(verDir); err != nil {
		stale := filepath.Join(i.cfg.Root, "tmp", "stale", candID+"-"+ver)
		if rerr := os.Rename(verDir, stale); rerr == nil {
			i.Warn("版本目录被占用，已移至 %s，将在下次清理", stale)
			return nil
		}
		return fmt.Errorf("卸载失败: %w", err)
	}
	return nil
}

// CleanStale 清理历史遗留的暂存/待删除文件。
func (i *Installer) CleanStale() {
	stale := filepath.Join(i.cfg.Root, "tmp", "stale")
	if entries, err := os.ReadDir(stale); err == nil {
		for _, e := range entries {
			os.RemoveAll(filepath.Join(stale, e.Name()))
		}
	}
	tmp := filepath.Join(i.cfg.Root, "tmp")
	if entries, err := os.ReadDir(tmp); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".delete-") {
				os.RemoveAll(filepath.Join(tmp, e.Name()))
			}
		}
	}
}

func applyMirrors(cfg *config.Config, urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		out = append(out, cfg.ApplyMirror(u))
	}
	return out
}
