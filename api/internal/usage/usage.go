// Package usage measures logical file sizes without counting deployment staging
// or nginx logs against a site's storage quota.
package usage

import (
	"io/fs"
	"os"
	"path/filepath"

	"hosting/internal/store"
)

type Breakdown struct {
	SiteBytes  int64 `json:"site_bytes"`
	DataBytes  int64 `json:"data_bytes"`
	LogBytes   int64 `json:"log_bytes"`
	TotalBytes int64 `json:"total_bytes"`
}

func size(root, skip string, optional bool) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if optional && path == root && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() && d.Name() == skip {
			return filepath.SkipDir
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func Measure(dataDir string, p *store.Project) (Breakdown, error) {
	var b Breakdown
	var err error
	b.SiteBytes, err = size(p.Path, ".conf", false)
	if err != nil {
		return b, err
	}
	for _, name := range []string{"access.log", "error.log"} {
		info, err := os.Lstat(filepath.Join(p.Path, ".conf", name))
		if err != nil && !os.IsNotExist(err) {
			return b, err
		}
		if err == nil && info.Mode().IsRegular() {
			b.LogBytes += info.Size()
		}
	}
	if p.Provider == store.ProviderPHP {
		b.DataBytes, err = size(store.ProjectDataRoot(dataDir, p), "", true)
		if err != nil {
			return b, err
		}
	}
	b.TotalBytes = b.SiteBytes + b.DataBytes
	return b, nil
}
