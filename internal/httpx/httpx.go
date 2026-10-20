package httpx

import (
	"context"
	"encoding/json"
	"net/http"

	"hosting/internal/store"
)

type ctxKey int

const ctxUser ctxKey = iota

func SetUser(ctx context.Context, u *store.User) context.Context {
	return context.WithValue(ctx, ctxUser, u)
}

func UserFrom(r *http.Request) *store.User {
	u, _ := r.Context().Value(ctxUser).(*store.User)
	return u
}

func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func ReadJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func Error(w http.ResponseWriter, code int, msg string) {
	WriteJSON(w, code, map[string]string{"error": msg})
}

func ErrStatus(err error) int {
	switch err {
	case store.ErrNotFound:
		return http.StatusNotFound
	case store.ErrDuplicate:
		return http.StatusConflict
	case store.ErrQuotaExceeded:
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusInternalServerError
	}
}