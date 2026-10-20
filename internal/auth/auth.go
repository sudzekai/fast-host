package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"hosting/internal/httpx"
	"hosting/internal/store"
)

const (
	CookieName = "fasthost_token"
	TokenTTL   = 12 * time.Hour
)

type Claims struct {
	UserID   int64  `json:"uid"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func Sign(secret string, u *store.User, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   u.Username,
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func Parse(secret, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	t, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !t.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(TokenTTL.Seconds()),
	})
}

func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func Middleware(secret string, st store.Storage, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := ""
			if c, err := r.Cookie(CookieName); err == nil {
				tokenStr = c.Value
			}
			if tokenStr == "" {
				if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
					tokenStr = strings.TrimPrefix(h, "Bearer ")
				}
			}
			if tokenStr == "" {
				httpx.Error(w, http.StatusUnauthorized, "требуется авторизация")
				return
			}
			claims, err := Parse(secret, tokenStr)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "недействительный токен")
				return
			}
			u, err := st.GetUserByID(claims.UserID)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "пользователь не найден")
				return
			}
			if len(roles) > 0 && !contains(roles, u.Role) {
				httpx.Error(w, http.StatusForbidden, "недостаточно прав")
				return
			}
			next.ServeHTTP(w, r.WithContext(httpx.SetUser(r.Context(), u)))
		})
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}