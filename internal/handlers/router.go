package handlers

import (
	"bytes"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"hosting/internal/auth"
	"hosting/internal/store"
)

func NewRouter(h *Handlers, secret string) http.Handler {
	mux := http.NewServeMux()

	authMW := auth.Middleware(secret, h.store)
	adminMW := auth.Middleware(secret, h.store, store.RoleAdmin)

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/register", h.register)
	mux.Handle("POST /api/auth/logout", authMW(http.HandlerFunc(h.logout)))

	mux.Handle("GET /api/me", authMW(http.HandlerFunc(h.me)))
	mux.Handle("PUT /api/users/me/password", authMW(http.HandlerFunc(h.changeOwnPassword)))

	mux.Handle("GET /api/projects", authMW(http.HandlerFunc(h.listProjects)))
	mux.Handle("POST /api/projects", authMW(http.HandlerFunc(h.createProject)))
	mux.Handle("GET /api/projects/{id}", authMW(http.HandlerFunc(h.getProject)))
	mux.Handle("DELETE /api/projects/{id}", authMW(http.HandlerFunc(h.deleteProject)))
	mux.Handle("POST /api/projects/{id}/upload", authMW(http.HandlerFunc(h.uploadProjectZip)))
	mux.Handle("GET /api/projects/{id}/download", authMW(http.HandlerFunc(h.downloadProjectZip)))
	mux.Handle("GET /api/projects/{id}/files", authMW(http.HandlerFunc(h.listFiles)))
	mux.Handle("GET /api/projects/{id}/metrics", authMW(http.HandlerFunc(h.projectMetrics)))
	mux.Handle("GET /api/projects/{id}/nginx-config", adminMW(http.HandlerFunc(h.getNginxConfig)))
	mux.Handle("POST /api/projects/{id}/apply-ssl", authMW(http.HandlerFunc(h.applySSL)))
	mux.Handle("PUT /api/projects/{id}/enabled", authMW(http.HandlerFunc(h.setProjectEnabled)))

	mux.Handle("GET /api/admin/users", adminMW(http.HandlerFunc(h.adminListUsers)))
	mux.Handle("GET /api/admin/users/{id}", adminMW(http.HandlerFunc(h.adminGetUser)))
	mux.Handle("PUT /api/admin/users/{id}/password", adminMW(http.HandlerFunc(h.adminSetPassword)))
	mux.Handle("PUT /api/admin/users/{id}/quota", adminMW(http.HandlerFunc(h.adminSetQuota)))
	mux.Handle("PUT /api/admin/registration", adminMW(http.HandlerFunc(h.adminSetRegistration)))
	mux.Handle("GET /api/admin/stats", adminMW(http.HandlerFunc(h.adminStats)))

	mux.Handle("/", staticHandler(h.web))
	return withLogging(mux)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/api/health" {
			log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}

func staticHandler(sub fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			p = "index.html"
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ct := mime.TypeByExtension(filepath.Ext(p))
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		http.ServeContent(w, r, p, time.Time{}, bytes.NewReader(b))
	})
}