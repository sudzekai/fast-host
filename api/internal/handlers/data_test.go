package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hosting/internal/config"
	"hosting/internal/httpx"
	"hosting/internal/store"
)

func TestProjectDataAccessAndRootContainment(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewStorage("json", filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := &store.Project{UserID: 1, Domain: "data.test", Path: filepath.Join(dir, "users/alice/projects/site"), Provider: store.ProviderPHP, DataDirs: []store.DataDirectory{{Name: "database", Env: "APP_DATA_DIR"}}}
	if err := st.CreateProject(p); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(store.ProjectDataRoot(dir, p), "volumes/database")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.json"), []byte(`{"origin":"https://example.test"}`), 0600); err != nil {
		t.Fatal(err)
	}
	h := New(&config.Config{DataDir: dir, PHPVersion: "8.4"}, st, nil, nil)
	request := func(userID int64, filename string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.SetPathValue("id", "1")
		r.SetPathValue("directory", "database")
		r.SetPathValue("file", filename)
		r = r.WithContext(httpx.SetUser(r.Context(), &store.User{ID: userID, Role: store.RoleUser}))
		w := httptest.NewRecorder()
		h.dataFile(w, r)
		return w
	}
	if got := request(2, "config.json"); got.Code != 403 {
		t.Fatalf("another user: %d", got.Code)
	}
	if got := request(1, "../outside"); got.Code != 400 {
		t.Fatalf("traversal: %d", got.Code)
	}
	got := request(1, "config.json")
	if got.Code != 200 {
		t.Fatalf("owner: %d %s", got.Code, got.Body.String())
	}
	var content map[string]string
	if err := json.Unmarshal(got.Body.Bytes(), &content); err != nil || !strings.Contains(content["content"], "example.test") {
		t.Fatalf("content: %s", got.Body.String())
	}
	outside := filepath.Join(dir, "secret.json")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dataDir, "link.json")); err != nil {
		t.Logf("symlink check unavailable: %v", err)
		return
	}
	if got := request(1, "link.json"); got.Code == 200 || strings.Contains(got.Body.String(), "secret") {
		t.Fatalf("escaped data root: %d %s", got.Code, got.Body.String())
	}
}
