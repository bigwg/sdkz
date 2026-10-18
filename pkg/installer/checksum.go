package installer

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strings"

	"sdkz/pkg/download"
)

// ErrChecksumNotFound 表示校验清单存在，但其中没有与下载文件对应的哈希值
// （如多平台清单 SHASUMS256.txt 中缺少目标文件，或镜像同步不同步）。
var ErrChecksumNotFound = errors.New("校验清单中未找到对应文件的哈希")

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
// filename 为下载文件的基线名（如 node-v24.22.0-linux-x64.tar.gz）：
// 当校验文件为多文件清单（如 Node.js SHASUMS256.txt 按平台列出全部哈希）时，
// 必须按文件名定位对应哈希；单值文件（裸哈希，如 Adoptium/Maven/Gradle 的
// .sha256/.sha512）直接返回。
func FetchChecksum(ctx context.Context, client *http.Client, url, algo, filename string) (string, error) {
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
	return parseChecksum(string(data), algo, filename)
}

// checksumEntry 校验清单中的一条记录。
type checksumEntry struct {
	hash string
	name string // 清单中该哈希对应的文件名（裸哈希文件为空）
}

// parseChecksum 从文本中解析校验值。
// 支持格式：
//   - "哈希  文件名"（每行一个文件，如 SHASUMS256.txt）
//   - "*哈希  文件名"（BSD 风格）
//   - 裸哈希（单值文件，如 .sha256/.sha512）
//
// 优先按 filename 匹配；匹配不到时，仅当文件为单值裸哈希才返回该值，
// 否则返回 ErrChecksumNotFound（防止多平台清单错取第一行）。
func parseChecksum(text, algo, filename string) (string, error) {
	wantLen := 64
	if algo == "sha512" {
		wantLen = 128
	}
	var entries []checksumEntry
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		cand := strings.TrimPrefix(strings.TrimSpace(fields[0]), "*")
		if len(cand) != wantLen || !isHex(cand) {
			continue
		}
		e := checksumEntry{hash: cand}
		if len(fields) >= 2 {
			// 取行末字段作为文件名（兼容 "哈希  文件名" 与 "*哈希  文件名"）。
			e.name = strings.TrimPrefix(fields[len(fields)-1], "*")
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("无法从校验值文件中解析 %s 值", algo)
	}
	// 1) 多文件清单：按下载文件名精确匹配（大小写不敏感）。
	if filename != "" {
		for _, e := range entries {
			if e.name != "" && strings.EqualFold(e.name, filename) {
				return e.hash, nil
			}
		}
	}
	// 2) 单值文件（无文件名部分的裸哈希）：直接采用。
	if len(entries) == 1 && entries[0].name == "" {
		return entries[0].hash, nil
	}
	// 3) 清单存在但找不到目标文件的哈希：报错而非错取其他文件的哈希。
	if filename != "" {
		return "", fmt.Errorf("%w: %s", ErrChecksumNotFound, filename)
	}
	return "", fmt.Errorf("%w（清单含 %d 个文件的哈希，无法确定目标）", ErrChecksumNotFound, len(entries))
}

func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}
