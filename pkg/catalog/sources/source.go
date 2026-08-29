// Package sources 实现各类远程版本清单适配器。
package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// base 提供基础 URL 覆盖能力（测试注入 / 特殊镜像）。
type base struct {
	base string
}

// SetBaseURL 覆盖基础 URL（仅当非空时）。
func (b *base) SetBaseURL(u string) {
	if u != "" {
		b.base = strings.TrimRight(u, "/")
	}
}

// join 拼接完整 URL；未覆盖时 path 必须是绝对 URL。
func (b *base) join(path string) string {
	if b.base != "" {
		return b.base + path
	}
	return path
}

// getJSON 发起 GET 并解码 JSON。
func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "sdkz/1.0")
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

// getText 发起 GET 并返回文本内容（限长）。
func getText(ctx context.Context, client *http.Client, url string, limit int64) (string, error) {
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
		io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("请求 %s 返回 %s", url, resp.Status)
	}
	if limit <= 0 {
		limit = 4 << 20
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return string(data), err
}

// JavaMajors 返回 Java 主要版本列表（来自 Adoptium available_releases；失败时回退内置列表）。
func JavaMajors(ctx context.Context, client *http.Client) []int {
	fallback := []int{8, 11, 17, 21}
	var info struct {
		AvailableReleases []int `json:"available_releases"`
	}
	if err := getJSON(ctx, client, "https://api.adoptium.net/v3/info/available_releases", &info); err != nil {
		return fallback
	}
	if len(info.AvailableReleases) == 0 {
		return fallback
	}
	return info.AvailableReleases
}

// httpClientWithTimeout 返回带超时的客户端（用于独立小请求）。
func httpClientWithTimeout() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// parallelFetch 并发执行 fn（并发数上限由 limit 控制），
// 全部成功返回 nil，任一失败返回首个错误。
func parallelFetch[T any](ctx context.Context, items []T, limit int, fn func(T) error) error {
	if len(items) == 0 {
		return nil
	}
	sem := make(chan struct{}, limit)
	errc := make(chan error, len(items))
	var wg sync.WaitGroup
	for _, it := range items {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		wg.Add(1)
		go func(x T) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fn(x); err != nil {
				errc <- err
			}
		}(it)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-errc:
		return err
	default:
		return nil
	}
}
