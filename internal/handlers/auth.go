package handlers

import (
	"net/http"

	"hosting/internal/auth"
	"hosting/internal/httpx"
	"hosting/internal/store"
)

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerReq struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	u, err := h.store.GetUserByUsername(req.Username)
	if err != nil || !auth.CheckPassword(u.PasswordHash, req.Password) {
		httpx.Error(w, http.StatusUnauthorized, "неверный логин или пароль")
		return
	}
	token, err := auth.Sign(h.cfg.JWTSecret, u, auth.TokenTTL)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка авторизации")
		return
	}
	auth.SetCookie(w, r, token)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	auth.ClearCookie(w, r)
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handlers) registrationEnabled() bool {
	if !h.cfg.AllowRegistration {
		return false
	}
	v, err := h.store.GetSetting("allow_registration")
	if err != nil {
		return true
	}
	return v == "1"
}

func (h *Handlers) register(w http.ResponseWriter, r *http.Request) {
	if !h.registrationEnabled() {
		httpx.Error(w, http.StatusForbidden, "регистрация отключена администратором")
		return
	}
	var req registerReq
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	if !validUsername(req.Username) {
		httpx.Error(w, http.StatusBadRequest, "логин: 3-32 символа, буквы/цифры/подчёркивание")
		return
	}
	if !validEmail(req.Email) {
		httpx.Error(w, http.StatusBadRequest, "укажите корректный email")
		return
	}
	if len(req.Password) < 6 {
		httpx.Error(w, http.StatusBadRequest, "пароль не короче 6 символов")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка хеширования")
		return
	}
	defQuota := store.DefaultQuotaBytes
	if v, err := h.store.GetSetting("default_quota"); err == nil {
		if n, ok := parseBytes(v); ok {
			defQuota = n
		}
	}
	u := &store.User{Username: req.Username, Email: req.Email, PasswordHash: hash, Role: store.RoleUser, QuotaBytes: defQuota}
	if err := h.store.CreateUser(u); err != nil {
		httpx.Error(w, httpx.ErrStatus(err), "пользователь с таким логином уже существует")
		return
	}
	token, err := auth.Sign(h.cfg.JWTSecret, u, auth.TokenTTL)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка авторизации")
		return
	}
	auth.SetCookie(w, r, token)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"user": u})
}

func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	u := httpx.UserFrom(r)
	projects, err := h.store.ListProjectsByUser(u.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	var used int64
	for _, p := range projects {
		used += p.DiskBytes
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user":         u,
		"used_bytes":   used,
		"project_count": len(projects),
	})
}

type changePasswordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *Handlers) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	u := httpx.UserFrom(r)
	var req changePasswordReq
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.CurrentPassword) {
		httpx.Error(w, http.StatusBadRequest, "текущий пароль неверен")
		return
	}
	if len(req.NewPassword) < 6 {
		httpx.Error(w, http.StatusBadRequest, "новый пароль не короче 6 символов")
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка хеширования")
		return
	}
	if err := h.store.UpdateUserPassword(u.ID, hash); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func parseBytes(s string) (int64, bool) {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return n, n > 0
}