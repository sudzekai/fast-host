package handlers

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"hosting/internal/config"
	"hosting/internal/httpx"
	"hosting/internal/metrics"
	"hosting/internal/store"
)

type Handlers struct {
	cfg     *config.Config
	store   store.Storage
	sampler *metrics.Sampler
	web     fs.FS
}

func New(cfg *config.Config, st store.Storage, sampler *metrics.Sampler, webFS fs.FS) *Handlers {
	return &Handlers{cfg: cfg, store: st, sampler: sampler, web: webFS}
}

var (
	nameRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)
	userRe   = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
)

func validProjectName(n string) bool { return nameRe.MatchString(n) }

func validUsername(n string) bool { return userRe.MatchString(n) }

func projectDir(dataDir, username, projectName string) string {
	return filepath.Join(dataDir, "users", username, "projects", projectName)
}

func cleanProjectPath(p *store.Project, path string) bool {
	root := filepath.Clean(p.Path)
	full := filepath.Clean(filepath.Join(p.Path, path))
	return strings.HasPrefix(full, root+string(os.PathSeparator)) || full == root
}

func (h *Handlers) canAccess(r *http.Request, p *store.Project) bool {
	u := httpx.UserFrom(r)
	return u.Role == store.RoleAdmin || u.ID == p.UserID
}

func (h *Handlers) loadProject(w http.ResponseWriter, r *http.Request) *store.Project {
	idStr := r.PathValue("id")
	p, err := h.store.GetProjectByID(parseID(idStr))
	if err != nil {
		httpx.Error(w, httpx.ErrStatus(err), "проект не найден")
		return nil
	}
	if !h.canAccess(r, p) {
		httpx.Error(w, http.StatusForbidden, "нет доступа к проекту")
		return nil
	}
	return p
}

func parseID(s string) int64 {
	var id int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		id = id*10 + int64(c-'0')
	}
	return id
}