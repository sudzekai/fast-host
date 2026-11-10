package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONCorruptMetricsAreNotSilentlyOverwritten(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &Project{Domain: "corrupt.test"}
	if err := s.CreateProject(p); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "metrics", "1.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LastMetric(p.ID); err == nil {
		t.Fatal("corrupt metrics reported as empty")
	}
	if err := s.AddMetric(&Metric{ProjectID: p.ID}); err == nil {
		t.Fatal("corrupt metrics overwritten")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "broken" {
		t.Fatal("original data lost")
	}
}

func TestJSONProjectDirectoriesArePersistedAndCopied(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := &Project{Domain: "php.test", Provider: ProviderPHP, DataDirs: []DataDirectory{{Name: "database", Env: "APP_DATA_DIR"}}}
	if err := s.CreateProject(p); err != nil {
		t.Fatal(err)
	}
	p.DataDirs[0].Env = "MUTATED_DIR"
	got, err := s.GetProjectByID(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DataDirs[0].Env != "APP_DATA_DIR" {
		t.Fatal("caller mutated stored directories")
	}
	got.DataDirs[0].Env = "OTHER_DIR"
	reloaded, err := NewJSONStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err = reloaded.GetProjectByID(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != ProviderPHP || got.DataDirs[0].Env != "APP_DATA_DIR" {
		t.Fatalf("not persisted: %+v", got)
	}
}

func TestJSONUserUsageAfterProjectChanges(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alice", "bob", "empty"} {
		if err := s.CreateUser(&User{Username: name}); err != nil {
			t.Fatal(err)
		}
	}
	for i, owner := range []int64{1, 2, 1} {
		if err := s.CreateProject(&Project{UserID: owner, Domain: fmt.Sprintf("%d.test", i), DiskBytes: 10}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpdateProjectDisk(1, 25); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(3); err != nil {
		t.Fatal(err)
	}
	s, err = NewJSONStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	users, err := s.ListUsersDetailed()
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 {
		t.Fatalf("users: %d", len(users))
	}
	for i, want := range []struct {
		bytes int64
		count int
	}{{25, 1}, {10, 1}, {0, 0}} {
		if users[i].UsedBytes != want.bytes || users[i].ProjectCount != want.count {
			t.Errorf("user %d: usage=%d count=%d, want %+v", users[i].ID, users[i].UsedBytes, users[i].ProjectCount, want)
		}
	}
}

// Регрессия: записи хранились в срезе значений, а мапы держали указатели на его элементы.
// После append (перевыделение) и удаления (сдвиг) указатели смотрели не туда: квота
// не сохранялась на диск, а GetProjectByID мог вернуть чужой проект.
func TestJSONStorePointersSurviveAppendAndDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"admin", "bob", "carol", "dave", "eve"} {
		if err := s.CreateUser(&User{Username: name, Role: RoleUser, QuotaBytes: DefaultQuotaBytes}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpdateUserQuota(2, 2<<30); err != nil {
		t.Fatal(err)
	}
	for i, d := range []string{"a.test", "b.test", "c.test"} {
		if err := s.CreateProject(&Project{UserID: 2, Name: string(rune('a' + i)), Domain: d, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteProject(1); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetProjectByID(2)
	if err != nil || p.Domain != "b.test" {
		t.Fatalf("project 2 after delete: %+v, %v", p, err)
	}
	p.Enabled = false
	p.UpdatedAt = "2026-01-01 00:00:00"
	if err := s.UpdateProject(p); err != nil {
		t.Fatal(err)
	}

	re, err := NewJSONStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := re.GetUserByID(2)
	if u.QuotaBytes != 2<<30 {
		t.Fatalf("quota not persisted: %d", u.QuotaBytes)
	}
	p2, _ := re.GetProjectByID(2)
	if p2.Enabled || p2.UpdatedAt != "2026-01-01 00:00:00" {
		t.Fatalf("project fields not persisted: %+v", p2)
	}
	p3, _ := re.GetProjectByID(3)
	if !p3.Enabled || p3.Domain != "c.test" {
		t.Fatalf("project 3 corrupted: %+v", p3)
	}
	if err := re.CreateProject(&Project{UserID: 2, Name: "d", Domain: "d.test"}); err != nil {
		t.Fatal(err)
	}
	if p4, _ := re.GetProjectByDomain("d.test"); p4.ID != 4 {
		t.Fatalf("id reused after reload: %d", p4.ID)
	}
}
