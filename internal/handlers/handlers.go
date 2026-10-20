package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"hosting/internal/config"
	"hosting/internal/httpx"
	"hosting/internal/metrics"
	"hosting/internal/nginx"
	"hosting/internal/store"
)

type Handlers struct {
	cfg     *config.Config
	store   store.Storage
	sampler *metrics.Sampler
	nginx   *nginx.Manager
}

func New(cfg *config.Config, st store.Storage, sampler *metrics.Sampler, ngx *nginx.Manager) *Handlers {
	return &Handlers{cfg: cfg, store: st, sampler: sampler, nginx: ngx}
}

var (
	nameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,63}$`)
	userRe  = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]{2,}$`)
)

func validProjectName(n string) bool { return nameRe.MatchString(n) }

func validUsername(n string) bool { return userRe.MatchString(n) }

func validEmail(e string) bool { return len(e) <= 254 && emailRe.MatchString(e) }

func projectDir(dataDir, username, projectName string) string {
	return filepath.Join(dataDir, "users", username, "projects", projectName)
}

// insideUsersDir защищает os.RemoveAll от пути вне {dataDir}/users (битая запись в БД и т.п.).
func (h *Handlers) insideUsersDir(path string) bool {
	root := filepath.Join(h.cfg.DataDir, "users") + string(os.PathSeparator)
	return path != "" && strings.HasPrefix(filepath.Clean(path)+string(os.PathSeparator), root) &&
		filepath.Clean(path)+string(os.PathSeparator) != root
}

func isAdmin(r *http.Request) bool {
	u := httpx.UserFrom(r)
	return u != nil && u.Role == store.RoleAdmin
}

// publicProject — то, что уходит в API: путь на диске виден только администратору.
func (h *Handlers) publicProject(r *http.Request, p *store.Project) map[string]any {
	out := map[string]any{
		"id": p.ID, "user_id": p.UserID, "username": p.Username, "name": p.Name, "domain": p.Domain,
		"status": p.Status, "enabled": p.Enabled, "ssl_enabled": p.SSLEnabled,
		"ssl_issued_at": p.SSLIssuedAt, "ssl_expires_at": p.SSLExpiresAt,
		"disk_bytes": p.DiskBytes, "created_at": p.CreatedAt, "updated_at": p.UpdatedAt,
		"site_url": h.siteURL(p),
	}
	if isAdmin(r) {
		out["path"] = p.Path
	}
	return out
}

func (h *Handlers) publicProjects(r *http.Request, list []store.Project) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for i := range list {
		out = append(out, h.publicProject(r, &list[i]))
	}
	return out
}

// hidePaths убирает из текста ошибки абсолютные пути сервера для клиентов.
func (h *Handlers) hidePaths(r *http.Request, msg string) string {
	if isAdmin(r) {
		return msg
	}
	return strings.ReplaceAll(msg, h.cfg.DataDir+string(os.PathSeparator), "")
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
	if owner, err := h.store.GetUserByID(p.UserID); err == nil {
		p.Username = owner.Username
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
