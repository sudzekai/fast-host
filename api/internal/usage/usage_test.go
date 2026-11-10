package usage

import (
	"hosting/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestStorageExcludesLogsAndStaging(t *testing.T) {
	root := t.TempDir()
	p := &store.Project{Path: filepath.Join(root, "site"), Provider: store.ProviderPHP}
	files := map[string]string{
		filepath.Join(p.Path, "index.php"):                                     "12345",
		filepath.Join(p.Path, ".conf/access.log"):                              "1234567890",
		filepath.Join(p.Path, ".conf/.upload-staging/index.php"):               "staging",
		filepath.Join(store.ProjectDataRoot(root, p), "volumes/db/app.sqlite"): "1234567",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	b, err := Measure(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if b.SiteBytes != 5 || b.DataBytes != 7 || b.LogBytes != 10 || b.TotalBytes != 12 {
		t.Fatalf("wrong breakdown: %+v", b)
	}
	if _, err := Measure(root, &store.Project{Path: filepath.Join(root, "missing")}); err == nil {
		t.Fatal("missing directory silently measured as empty")
	}
}
