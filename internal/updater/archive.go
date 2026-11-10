package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxBinarySize guards against a malformed or oversized archive entry.
const maxBinarySize = 200 << 20 // 200 MiB

// parseChecksums reads a `sha256sum` style index into an asset name lookup.
func parseChecksums(data []byte) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 {
			continue
		}
		// Binary-mode entries are prefixed with '*'.
		name := strings.TrimPrefix(fields[1], "*")
		out[strings.ToLower(filepath.Base(name))] = strings.ToLower(fields[0])
	}
	return out
}

// sha256File returns the lowercase hex digest of a local file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractBinary pulls the CLI executable out of a release archive. Archives
// published by the release workflow keep the binary at the root, entries deeper
// than one directory are ignored.
func extractBinary(archivePath, wantName, dstPath string) error {
	if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		return extractFromZip(archivePath, wantName, dstPath)
	}
	return extractFromTarGz(archivePath, wantName, dstPath)
}

func entryMatches(name, wantName string) bool {
	clean := filepath.ToSlash(name)
	if strings.Count(clean, "/") > 1 {
		return false
	}
	base := filepath.Base(clean)
	return base == wantName || strings.EqualFold(base, wantName)
}

func extractFromTarGz(archivePath, wantName, dstPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("解压 gzip 失败: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取 tar 归档失败: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg || !entryMatches(hdr.Name, wantName) {
			continue
		}
		return writeLimited(tr, dstPath)
	}

	return fmt.Errorf("归档中未找到可执行文件 %q", wantName)
}

func extractFromZip(archivePath, wantName, dstPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("打开 zip 归档失败: %w", err)
	}
	defer zr.Close()

	for _, zf := range zr.File {
		if !entryMatches(zf.Name, wantName) {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return fmt.Errorf("解压 zip 条目失败: %w", err)
		}
		err = writeLimited(rc, dstPath)
		rc.Close()
		if err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("归档中未找到可执行文件 %q", wantName)
}

func writeLimited(r io.Reader, dstPath string) error {
	// The entry name is never used to build dstPath, so a traversal payload
	// such as "../nga" cannot redirect the write.
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	n, err := io.Copy(dst, io.LimitReader(r, maxBinarySize+1))
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("写出可执行文件失败: %w", err)
	}
	if n > maxBinarySize {
		return fmt.Errorf("可执行文件超过 %d MiB 上限，已中止", maxBinarySize>>20)
	}
	return os.Chmod(dstPath, 0755)
}
