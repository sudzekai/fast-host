package phpfpm

import (
	"strings"
	"testing"

	"hosting/internal/store"
)

func TestRenderPoolSeparatesProjectData(t *testing.T) {
	p := &store.Project{Path: "/srv/users/alice/projects/site", DataDirs: []store.DataDirectory{{Name: "database", Env: "BROSAYU_DATA_DIR"}}}
	text, err := Render(p, "/srv/data/project-data/pool", "/run/php/pool.sock")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"user = " + store.ProjectKey(p), "group = " + store.ProjectKey(p),
		`env[BROSAYU_DATA_DIR] = "/srv/data/project-data/pool/volumes/database"`,
		"clear_env = yes", "listen.mode = 0660", "security.limit_extensions = .php", "pm = ondemand",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "/srv/users/bob") {
		t.Fatal("another project's directory allowed")
	}
}

func TestRenderRejectsUnsafeAndDuplicateMappings(t *testing.T) {
	for _, dirs := range [][]store.DataDirectory{
		{{Name: "../escape", Env: "APP_DATA_DIR"}},
		{{Name: "db", Env: "APP_DATA_DIR]\nenv[EVIL"}},
		{{Name: "db", Env: "APP_DATA_DIR"}, {Name: "db", Env: "OTHER_DIR"}},
		{{Name: "db", Env: "APP_DATA_DIR"}, {Name: "other", Env: "APP_DATA_DIR"}},
	} {
		if _, err := Render(&store.Project{Path: "/srv/site", DataDirs: dirs}, "/srv/data", "/run/php/pool.sock"); err == nil {
			t.Errorf("accepted %+v", dirs)
		}
	}
	if _, err := Render(&store.Project{Path: "/srv/$HOME/site"}, "/srv/data", "/run/php/pool.sock"); err == nil {
		t.Fatal("accepted FPM variable expansion")
	}
}
