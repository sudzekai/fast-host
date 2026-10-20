//go:build db_sql

package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// Старая схема без enabled/ssl_*/updated_at должна догоняться миграцией.
func TestSQLiteMigratesOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []string{
		`CREATE TABLE projects (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id BIGINT NOT NULL, name VARCHAR(191) NOT NULL,
			domain VARCHAR(191) NOT NULL, status VARCHAR(32) NOT NULL DEFAULT 'active', ssl_enabled INTEGER NOT NULL DEFAULT 0,
			path TEXT NOT NULL, disk_bytes BIGINT NOT NULL DEFAULT 0, created_at TEXT NOT NULL, UNIQUE (domain))`,
		`INSERT INTO projects (user_id, name, domain, path, created_at) VALUES (1, 'old', 'old.test', '/x', '2026-01-01 00:00:00')`,
	} {
		if _, err := db.Exec(st); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	s, err := NewSQLStorage("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.GetProjectByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Enabled {
		t.Fatal("old project must be enabled after migration")
	}
	p.Enabled, p.SSLExpiresAt, p.UpdatedAt = false, "2027-01-01 00:00:00", "2026-02-02 00:00:00"
	if err := s.UpdateProject(p); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting("k", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting("k", "v2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetSetting("k"); v != "v2" {
		t.Fatalf("setting: %q", v)
	}
	list, err := s.ListAllProjects()
	if err == nil && len(list) != 0 {
		t.Fatalf("join without users must be empty, got %d", len(list))
	}
	got, _ := s.GetProjectByID(1)
	if got.Enabled || got.SSLExpiresAt != "2027-01-01 00:00:00" || got.UpdatedAt != "2026-02-02 00:00:00" {
		t.Fatalf("not persisted: %+v", got)
	}
	if _, err := s.ListMetrics(1, 10); err != nil {
		t.Fatal(err)
	}
}
