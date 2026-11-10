package handlers

import (
	"io"
	"net/http"

	"hosting/internal/httpx"
	"hosting/internal/store"
)

// apiKeyMW аутентифицирует по заголовку X-Api-Key (деплой из GitHub Actions).
func apiKeyMW(st store.Storage) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, err := st.GetUserByAPIKey(r.Header.Get("X-Api-Key"))
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "недействительный API-ключ")
				return
			}
			next.ServeHTTP(w, r.WithContext(httpx.SetUser(r.Context(), u)))
		})
	}
}

func (h *Handlers) myAPIKey(w http.ResponseWriter, r *http.Request) {
	u := httpx.UserFrom(r)
	if u.APIKey == "" {
		key := store.NewAPIKey()
		if err := h.store.UpdateUserAPIKey(u.ID, key); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
			return
		}
		u.APIKey = key
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"api_key": u.APIKey})
}

func (h *Handlers) regenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	u := httpx.UserFrom(r)
	key := store.NewAPIKey()
	if err := h.store.UpdateUserAPIKey(u.ID, key); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"api_key": key})
}

// deploy принимает zip с файлами проекта по API-ключу: тело запроса — сам архив
// (Content-Type: application/zip) или multipart-поле file.
func (h *Handlers) deploy(w http.ResponseWriter, r *http.Request) {
	h.projectMu.Lock()
	defer h.projectMu.Unlock()
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	var file io.Reader
	if ct := r.Header.Get("Content-Type"); len(ct) >= 9 && ct[:9] == "multipart" {
		r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
		f, _, err := r.FormFile("file")
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "поле file (zip-архив) обязательно")
			return
		}
		defer f.Close()
		file = f
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
		file = r.Body
	}
	written, ok := h.receiveZip(w, p, file, true)
	if !ok {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"project":    h.publicProject(r, p),
		"written":    written,
		"disk_bytes": p.DiskBytes,
	})
}
