// Package download 提供带进度、断点续传与重试的 HTTP 下载。
package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ProgressFunc 下载进度回调（done/total 为字节数）。
type ProgressFunc func(done, total int64)

const (
	maxRetries = 3
)

// Download 下载 url 到 dest。
//   - 先下载到 dest+".part"，支持 Range 断点续传；
//   - 网络错误重试（指数退避）；
//   - 完成后原子 rename 到 dest。
func Download(ctx context.Context, client *http.Client, url, dest string, progress ProgressFunc) error {
	part := dest + ".part"
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			select {
			case <-time.After(time.Duration(attempt-1) * time.Second):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		lastErr = downloadOnce(ctx, client, url, part, progress)
		if lastErr == nil {
			if err := os.Rename(part, dest); err != nil {
				return fmt.Errorf("移动下载文件失败: %w", err)
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// 404 是确定性失败，重试无意义，直接交给上层尝试 fallback URL。
		if IsNotFound(lastErr) {
			return lastErr
		}
	}
	return fmt.Errorf("下载 %s 失败（已重试 %d 次）: %w", url, maxRetries, lastErr)
}

func downloadOnce(ctx context.Context, client *http.Client, url, part string, progress ProgressFunc) error {
	// 已有部分文件则续传。
	var resume int64
	if fi, err := os.Stat(part); err == nil {
		resume = fi.Size()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "sdkz/1.0")
	if resume > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resume))
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		if resume > 0 {
			// 服务器不支持断点，从头下载。
			if err := f.Truncate(0); err != nil {
				return err
			}
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			resume = 0
		}
	case http.StatusPartialContent:
		// 续传，文件指针已就绪。
		if _, err := f.Seek(resume, io.SeekStart); err != nil {
			return err
		}
	case http.StatusNotFound:
		return &NotFoundError{URL: url}
	default:
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength + resume
	buf := make([]byte, 256*1024)
	var done int64 = resume
	if progress != nil {
		progress(done, total)
	}
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			done += int64(n)
			if progress != nil {
				progress(done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if progress != nil {
		progress(done, total)
	}
	return nil
}

// NotFoundError 表示资源不存在（触发 fallback URL 尝试）。
type NotFoundError struct{ URL string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("资源不存在: %s", e.URL) }

// IsNotFound 判断错误是否为 404。
func IsNotFound(err error) bool {
	var nf *NotFoundError
	return errors.As(err, &nf)
}
