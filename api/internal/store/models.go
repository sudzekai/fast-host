package store

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

const (
	RoleAdmin = "admin"
	RoleUser  = "user"

	StatusActive = "active"
	StatusError  = "error"

	DefaultQuotaBytes = int64(5) * 1024 * 1024 * 1024
	MaxQuotaBytes     = int64(100) * 1024 * 1024 * 1024
	MinQuotaBytes     = int64(100) * 1024 * 1024
)

// TimeFormat — общий формат дат во всех драйверах.
const TimeFormat = "2006-01-02 15:04:05"

func Now() string { return time.Now().Format(TimeFormat) }

var (
	ErrNotFound      = errors.New("not found")
	ErrDuplicate     = errors.New("already exists")
	ErrQuotaExceeded = errors.New("quota exceeded")
)

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	QuotaBytes   int64  `json:"quota_bytes"`
	CreatedAt    string `json:"created_at"`
	APIKey       string `json:"api_key"`
}

// NewAPIKey генерирует уникальный ключ пользователя (UUID v4).
func NewAPIKey() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type UserDetailed struct {
	User
	UsedBytes    int64 `json:"used_bytes"`
	ProjectCount int   `json:"project_count"`
}

type Project struct {
	Provider     string          `json:"provider,omitempty"`
	DataDirs     []DataDirectory `json:"data_dirs,omitempty"`
	ID           int64           `json:"id"`
	UserID       int64           `json:"user_id"`
	Username     string          `json:"username"`
	Name         string          `json:"name"`
	Domain       string          `json:"domain"`
	Status       string          `json:"status"`
	Enabled      bool            `json:"enabled"`
	SSLEnabled   bool            `json:"ssl_enabled"`
	SSLIssuedAt  string          `json:"ssl_issued_at"`
	SSLExpiresAt string          `json:"ssl_expires_at"`
	Path         string          `json:"path,omitempty"`
	DiskBytes    int64           `json:"disk_bytes"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

type DataDirectory struct {
	Name string `json:"name"`
	Env  string `json:"env"`
}

type Metric struct {
	LogOffset    int64  `json:"-"`
	LogIdentity  string `json:"-"`
	ID           int64  `json:"id"`
	ProjectID    int64  `json:"project_id"`
	TS           string `json:"ts"`
	DiskBytes    int64  `json:"disk_bytes"`
	RequestsCum  int64  `json:"requests_cum"`
	BandwidthCum int64  `json:"bandwidth_cum"`
}
