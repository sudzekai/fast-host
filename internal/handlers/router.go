package handlers

import (
	"log"
	"net/http"
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

	return withLogging(cors(h.cfg.CORSOrigins, mux))
}

// cors разрешает запросы только с перечисленных origin (разделённых запятой).
// Креденшелы (cookie) передаются только если origin совпал.
func cors(origins []string, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range origins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(allowed) > 0 {
			origin := r.Header.Get("Origin")
			if allowed[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
			w.Header().Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
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