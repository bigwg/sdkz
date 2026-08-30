// Package selfupdate 实现 GitHub Releases 自更新。
package selfupdate

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"sdkz/pkg/download"
	"sdkz/pkg/installer"
)

// ProgressFunc 进度回调。
type ProgressFunc func(done, total int64, stage string)

const ghAPI = "https://api.github.com"

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// Update 从 repo（owner/repo）下载最新版本并替换当前二进制。
// 返回新版本号。
func Update(ctx context.Context, client *http.Client, repo string, prog ProgressFunc) (string, error) {
	if repo == "" {
		return "", fmt.Errorf("未配置 self_update_repo（config.toml 中的 owner/repo）")
	}
	var rel release
	url := fmt.Sprintf("%s/repos/%s/releases/latest", ghAPI, repo)
	if err := getJSON(ctx, client, url, &rel); err != nil {
		return "", fmt.Errorf("获取最新版本失败: %w", err)
	}
	platformName := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	// 命名约定：sdkz-{os}-{arch}.tar.gz / .zip
	var assetURL, assetName string
	checksumURL := ""
	for _, a := range rel.Assets {
		if strings.HasPrefix(a.Name, "sdkz-"+platformName) &&
			(strings.HasSuffix(a.Name, ".tar.gz") || strings.HasSuffix(a.Name, ".zip")) {
			assetURL = a.URL
			assetName = a.Name
		}
		if a.Name == "checksums.txt" {
			checksumURL = a.URL
		}
	}
	if assetURL == "" {
		return "", fmt.Errorf("发布版本 %s 中没有适用于 %s 的产物", rel.TagName, platformName)
	}

	tmpDir, err := os.MkdirTemp("", "sdkz-selfupdate-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	archive := filepath.Join(tmpDir, assetName)
	if err := download.Download(ctx, client, assetURL, archive, func(done, total int64) {
		if prog != nil {
			prog(done, total, "更新下载")
		}
	}); err != nil {
		return "", fmt.Errorf("下载更新失败: %w", err)
	}

	// 校验（checksums.txt 存在时）。
	if checksumURL != "" {
		text, err := fetchText(ctx, client, checksumURL)
		if err == nil {
			want := checksumFor(text, assetName)
			if want != "" {
				if err := installer.VerifyFile(archive, want, "sha256"); err != nil {
					return "", fmt.Errorf("更新包校验失败: %w", err)
				}
			}
		}
	}

	// 解压。
	stage := filepath.Join(tmpDir, "stage")
	if err := installer.Extract(ctx, archive, stage, 0); err != nil {
		return "", fmt.Errorf("解压更新包失败: %w", err)
	}
	newBin := filepath.Join(stage, "sdkz")
	if runtime.GOOS == "windows" {
		newBin = filepath.Join(stage, "sdkz.exe")
	}
	if _, err := os.Stat(newBin); err != nil {
		return "", fmt.Errorf("更新包中缺少可执行文件 sdkz")
	}

	// 替换当前二进制。
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := replace(exe, newBin); err != nil {
		return "", fmt.Errorf("替换二进制失败: %w", err)
	}
	return strings.TrimPrefix(rel.TagName, "v"), nil
}

// replace 原子替换二进制：旧文件改名保留，新文件就位。
func replace(exe, newBin string) error {
	dir := filepath.Dir(exe)
	tmp := filepath.Join(dir, ".sdkz-new")
	backup := filepath.Join(dir, ".sdkz-old")
	os.Remove(tmp)
	os.Remove(backup)

	if err := copyFile(newBin, tmp); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	// 旧二进制重命名（Windows 上运行中 exe 可重命名）。
	if err := os.Rename(exe, backup); err != nil {
		// 若无法改名（Unix 上被占用），直接覆盖式 rename。
		return os.Rename(tmp, exe)
	}
	if err := os.Rename(tmp, exe); err != nil {
		// 回滚。
		os.Rename(backup, exe)
		return err
	}
	os.Remove(backup)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}

// checksumFor 从 checksums.txt 文本中提取指定文件的校验值。
func checksumFor(text, name string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.HasSuffix(fields[1], name) {
			if _, err := hex.DecodeString(fields[0]); err == nil {
				return fields[0]
			}
		}
		if len(fields) >= 1 && strings.HasSuffix(line, "  "+name) {
			return strings.Fields(line)[0]
		}
	}
	return ""
}

func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "sdkz/1.0")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("请求 %s 返回 %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func fetchText(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sdkz/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(data), err
}
