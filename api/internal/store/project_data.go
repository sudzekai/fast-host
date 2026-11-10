package store

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
)

const ProviderNginx = "nginx"
const ProviderPHP = "php-fpm"

// Empty providers belong to legacy projects and retain the old global PHP setting.
func ProjectKey(p *Project) string {
	sum := sha256.Sum256([]byte(filepath.Clean(p.Path)))
	return fmt.Sprintf("fh%x", sum[:10])
}

func ProjectDataRoot(dataDir string, p *Project) string {
	root, _ := filepath.Abs(filepath.Join(dataDir, "data", "project-data", ProjectKey(p)))
	return root
}
