package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hosting/internal/config"
	"hosting/internal/httpx"
	"hosting/internal/metrics"
	"hosting/internal/store"
)

func TestMetricRefreshReportsFreshStaleAndFailedStates(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewStorage("json", filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := &store.Project{UserID: 1, Domain: "metrics.test", Path: filepath.Join(dir, "site")}
	if err := os.MkdirAll(filepath.Join(p.Path, ".conf"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Path, "index.html"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProject(p); err != nil {
		t.Fatal(err)
	}
	h := New(&config.Config{DataDir: dir, MetricsIntervalSec: 60}, st, metrics.New(st, dir, time.Minute), nil)
	call := func(refresh bool, owner int64) (int, map[string]any) {
		r := httptest.NewRequest("GET", "/", nil)
		r.SetPathValue("id", "1")
		r = r.WithContext(httpx.SetUser(r.Context(), &store.User{ID: owner, Role: store.RoleUser}))
		w := httptest.NewRecorder()
		if refresh {
			h.refreshProjectMetrics(w, r)
		} else {
			h.projectMetrics(w, r)
		}
		var data map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		return w.Code, data
	}
	_, data := call(false, 1)
	if data["status"] != "pending" {
		t.Fatalf("initial: %+v", data)
	}
	if code, _ := call(true, 2); code != 403 {
		t.Fatalf("non-owner refresh: %d", code)
	}
	code, data := call(true, 1)
	if code != 200 || data["status"] != "ok" || data["disk_bytes"] != float64(5) || data["updated_at"] == nil {
		t.Fatalf("refresh: %d %+v", code, data)
	}
	if data["storage"].(map[string]any)["site_bytes"] != float64(5) {
		t.Fatalf("missing breakdown: %+v", data)
	}
	if err := st.AddMetric(&store.Metric{ProjectID: p.ID, TS: time.Now().Add(-10 * time.Minute).Format(store.TimeFormat), DiskBytes: 5}); err != nil {
		t.Fatal(err)
	}
	_, data = call(false, 1)
	if data["status"] != "stale" {
		t.Fatalf("stale: %+v", data)
	}
	if err := os.RemoveAll(p.Path); err != nil {
		t.Fatal(err)
	}
	if code, _ := call(true, 1); code != 500 {
		t.Fatalf("failed refresh: %d", code)
	}
	_, data = call(false, 1)
	if data["status"] != "error" || data["disk_bytes"] != float64(5) {
		t.Fatalf("failure discarded previous data: %+v", data)
	}
}
