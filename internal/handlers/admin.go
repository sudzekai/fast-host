package handlers

import (
	"net/http"

	"hosting/internal/auth"
	"hosting/internal/httpx"
	"hosting/internal/store"
)

func (h *Handlers) adminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.ListUsersDetailed()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (h *Handlers) adminGetUser(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	ud, err := h.store.GetUserDetailed(id)
	if err != nil {
		httpx.Error(w, httpx.ErrStatus(err), "пользователь не найден")
		return
	}
	projects, err := h.store.ListProjectsByUser(id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	for i := range projects {
		projects[i].Username = ud.Username
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user":     ud,
		"projects": h.publicProjects(r, projects),
		"data_dir": h.cfg.DataDir,
	})
}

type adminPasswordReq struct {
	NewPassword string `json:"new_password"`
}

func (h *Handlers) adminSetPassword(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if _, err := h.store.GetUserByID(id); err != nil {
		httpx.Error(w, httpx.ErrStatus(err), "пользователь не найден")
		return
	}
	var req adminPasswordReq
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	if len(req.NewPassword) < 6 {
		httpx.Error(w, http.StatusBadRequest, "пароль не короче 6 символов")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка хеширования")
		return
	}
	if err := h.store.UpdateUserPassword(id, hash); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type quotaReq struct {
	QuotaBytes int64 `json:"quota_bytes"`
}

func (h *Handlers) adminSetQuota(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if _, err := h.store.GetUserByID(id); err != nil {
		httpx.Error(w, httpx.ErrStatus(err), "пользователь не найден")
		return
	}
	var req quotaReq
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	if req.QuotaBytes < store.MinQuotaBytes || req.QuotaBytes > store.MaxQuotaBytes {
		httpx.Error(w, http.StatusBadRequest, "квота от 100 МБ до 100 ГБ")
		return
	}
	if err := h.store.UpdateUserQuota(id, req.QuotaBytes); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type registrationReq struct {
	Enabled bool `json:"enabled"`
}

func (h *Handlers) adminSetRegistration(w http.ResponseWriter, r *http.Request) {
	var req registrationReq
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	val := "0"
	if req.Enabled {
		val = "1"
	}
	if err := h.store.SetSetting("allow_registration", val); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"enabled": req.Enabled})
}

func (h *Handlers) adminStats(w http.ResponseWriter, r *http.Request) {
	users, err := h.store.CountUsers()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	projects, err := h.store.ListAllProjects()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	var totalDisk int64
	for _, p := range projects {
		totalDisk += p.DiskBytes
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"users":                 users,
		"projects":              len(projects),
		"total_disk":            totalDisk,
		"data_dir":              h.cfg.DataDir,
		"registration_enabled":  h.registrationEnabled(),
		"registration_flag_off": !h.cfg.AllowRegistration,
	})
}
