package handlers

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"hosting/internal/store"
)

func extractZip(r io.ReaderAt, size int64, dest string, maxBytes int64) (written int64, err error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return 0, fmt.Errorf("не удалось открыть zip: %w", err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, err
	}
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if strings.Contains(name, "..") {
			return 0, fmt.Errorf("архив содержит небезопасный путь: %s", name)
		}
		name = strings.TrimPrefix(name, "/")
		if name == "" {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) {
			return 0, fmt.Errorf("архив содержит путь вне проекта: %s", name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return 0, err
			}
			continue
		}
		if f.UncompressedSize64 > uint64(maxBytes-written) {
			return 0, store.ErrQuotaExceeded
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return 0, err
		}
		rc, err := f.Open()
		if err != nil {
			return 0, err
		}
		dst, err := os.Create(target)
		if err != nil {
			rc.Close()
			return 0, err
		}
		n, cErr := io.CopyN(dst, rc, int64(f.UncompressedSize64))
		rc.Close()
		dst.Close()
		if cErr != nil {
			return 0, fmt.Errorf("ошибка распаковки %s: %w", name, cErr)
		}
		written += n
	}
	return written, nil
}

func hasIndex(projectDir string) bool {
	for _, name := range []string{"index.php", "index.html", "index.htm"} {
		if fi, err := os.Stat(filepath.Join(projectDir, name)); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func mergeDir(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		return os.Rename(p, target)
	})
}

// replaceDir заменяет файлы сайта содержимым staging: всё, кроме служебного
// каталога keep (".conf"), сначала переносится в резервную папку. Если перенос
// новых файлов сорвался, старые файлы возвращаются на место.
func replaceDir(src, dst, keep string) error {
	backup, err := os.MkdirTemp(filepath.Join(dst, keep), ".previous-deploy-*")
	if err != nil {
		return err
	}
	var moved []string
	entries, err := os.ReadDir(dst)
	if err != nil {
		os.RemoveAll(backup)
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == keep {
			continue
		}
		if err := os.Rename(filepath.Join(dst, name), filepath.Join(backup, name)); err != nil {
			for _, m := range moved {
				_ = os.Rename(filepath.Join(backup, m), filepath.Join(dst, m))
			}
			os.RemoveAll(backup)
			return err
		}
		moved = append(moved, name)
	}
	if err := mergeDir(src, dst); err != nil {
		if rest, readErr := os.ReadDir(dst); readErr == nil {
			for _, e := range rest {
				if e.Name() != keep {
					_ = os.RemoveAll(filepath.Join(dst, e.Name()))
				}
			}
		}
		for _, m := range moved {
			_ = os.Rename(filepath.Join(backup, m), filepath.Join(dst, m))
		}
		os.RemoveAll(backup)
		return err
	}
	os.RemoveAll(backup)
	return nil
}

var errNoIndex = errors.New("в архиве нет index.html, index.htm или index.php в корне проекта")
