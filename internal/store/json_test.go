package store

import "testing"

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
