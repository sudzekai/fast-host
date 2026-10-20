package store

import "errors"

const (
	RoleAdmin = "admin"
	RoleUser  = "user"

	StatusActive = "active"
	StatusError  = "error"

	DefaultQuotaBytes = int64(5) * 1024 * 1024 * 1024
	MaxQuotaBytes     = int64(100) * 1024 * 1024 * 1024
	MinQuotaBytes     = int64(100) * 1024 * 1024
)

var (
	ErrNotFound      = errors.New("not found")
	ErrDuplicate     = errors.New("already exists")
	ErrQuotaExceeded = errors.New("quota exceeded")
)

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	QuotaBytes   int64  `json:"quota_bytes"`
	CreatedAt    string `json:"created_at"`
}

type UserDetailed struct {
	User
	UsedBytes    int64 `json:"used_bytes"`
	ProjectCount int   `json:"project_count"`
}

type Project struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	Domain     string `json:"domain"`
	Status     string `json:"status"`
	SSLEnabled bool   `json:"ssl_enabled"`
	Path       string `json:"path"`
	DiskBytes  int64  `json:"disk_bytes"`
	CreatedAt  string `json:"created_at"`
}

type Metric struct {
	ID           int64  `json:"id"`
	ProjectID    int64  `json:"project_id"`
	TS           string `json:"ts"`
	DiskBytes    int64  `json:"disk_bytes"`
	RequestsCum  int64  `json:"requests_cum"`
	BandwidthCum int64  `json:"bandwidth_cum"`
}