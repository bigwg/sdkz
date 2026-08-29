package installer

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strings"

	"sdkz/pkg/download"
)

// VerifyFile 校验文件哈希（algo: sha256 / sha512）。
func VerifyFile(path, expectedHex, algo string) error {
	expected := strings.ToLower(strings.TrimSpace(expectedHex))
	if expected == "" {
		return nil
	}
	if len(expected) < 32 {
		return fmt.Errorf("校验值格式异常: %q", expectedHex)
	}
	var h hash.Hash
	switch strings.ToLower(algo) {
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		return fmt.Errorf("不支持的校验算法: %s", algo)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("校验失败: 期望 %s，实际 %s", expected, got)
	}
	return nil
}

// FetchChecksum 从 URL 下载校验值文件并解析。
// 文件格式支持：裸哈希 或 "哈希  文件名" 行。
func FetchChecksum(ctx context.Context, client *http.Client, url, algo string) (string, error) {
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
	if resp.StatusCode == http.StatusNotFound {
		return "", &download.NotFoundError{URL: url}
	}
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("获取校验值 %s 返回 %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return parseChecksum(string(data), algo)
}

// parseChecksum 从文本中解析校验值。
func parseChecksum(text, algo string) (string, error) {
	wantLen := 64
	if algo == "sha512" {
		wantLen = 128
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			cand := strings.TrimSpace(fields[0])
			// 兼容 "*hash  filename"（bsd 风格）。
			cand = strings.TrimPrefix(cand, "*")
			if len(cand) == wantLen && isHex(cand) {
				return cand, nil
			}
		}
	}
	return "", fmt.Errorf("无法从校验值文件中解析 %s 值", algo)
}

func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}
