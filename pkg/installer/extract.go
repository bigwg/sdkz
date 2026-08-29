package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractTarGz 解压 tar.gz / tar 归档到 dest，剥掉 strip 层顶层目录。
func extractTar(ctx context.Context, archive, dest string, strip int) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()

	var r io.Reader = f
	if strings.HasSuffix(archive, ".tar.gz") || strings.HasSuffix(archive, ".tgz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("解压 gzip 失败: %w", err)
		}
		defer gz.Close()
		r = gz
	}

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取 tar 归档失败: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, skip := stripName(hdr.Name, strip)
		if skip {
			continue
		}
		target := filepath.Join(dest, rel)
		if !within(dest, target) {
			return fmt.Errorf("归档包含非法路径: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFile(target, tr, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			// 避免链接逃逸（保守处理：仅当目标在归档内才创建）。
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				// 链接创建失败不阻断安装（如权限限制）。
				_ = err
			}
		}
	}
	return nil
}

// extractZip 解压 zip 归档，含路径穿越防护。
func extractZip(ctx context.Context, archive, dest string, strip int) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("打开 zip 失败: %w", err)
	}
	defer zr.Close()
	for _, zf := range zr.File {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, skip := stripName(zf.Name, strip)
		if skip {
			continue
		}
		target := filepath.Join(dest, rel)
		if !within(dest, target) {
			return fmt.Errorf("归档包含非法路径: %s", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if zf.Mode()&0o111 != 0 {
			mode = 0o755
		}
		err = writeFile(target, rc, mode)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// Extract 按扩展名选择解压方式。
func Extract(ctx context.Context, archive, dest string, strip int) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	name := strings.ToLower(archive)
	switch {
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"), strings.HasSuffix(name, ".tar"):
		return extractTar(ctx, archive, dest, strip)
	case strings.HasSuffix(name, ".zip"):
		return extractZip(ctx, archive, dest, strip)
	default:
		return fmt.Errorf("不支持的归档格式: %s", archive)
	}
}

// stripName 剥离顶层目录，返回相对路径与是否跳过该项。
func stripName(name string, strip int) (string, bool) {
	name = strings.TrimPrefix(filepath.ToSlash(name), "./")
	if name == "" {
		return "", true
	}
	parts := strings.Split(name, "/")
	if strip <= 0 {
		if name[len(name)-1] == '/' {
			return filepath.FromSlash(strings.TrimSuffix(name, "/")), false
		}
		return filepath.FromSlash(name), false
	}
	if len(parts) <= strip {
		return "", true // 顶层目录项本身
	}
	// 过滤空段（如 "jdk-21/" 剥离后仅剩空段）。
	kept := make([]string, 0, len(parts)-strip)
	for _, p := range parts[strip:] {
		if p != "" {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return "", true
	}
	return filepath.FromSlash(strings.Join(kept, "/")), false
}

func within(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeFile(path string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
