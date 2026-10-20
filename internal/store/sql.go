//go:build db_sql

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

type dialect struct {
	name      string
	autoInc   string
	tsType    string
	engineSuf string
}

type SQLStorage struct {
	db *sql.DB
	d  dialect
}

var _ Storage = (*SQLStorage)(nil)

func NewSQLStorage(driver, dsn string) (*SQLStorage, error) {
	var d dialect
	switch driver {
	case "sqlite":
		d = dialect{name: "sqlite", autoInc: "INTEGER PRIMARY KEY AUTOINCREMENT", tsType: "TEXT", engineSuf: ""}
		if !strings.HasPrefix(dsn, "file:") {
			dsn = "file:" + dsn
		}
		dsn += "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)"
	case "mysql":
		d = dialect{name: "mysql", autoInc: "BIGINT UNSIGNED NOT NULL AUTO_INCREMENT", tsType: "DATETIME", engineSuf: " ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci"}
	default:
		return nil, fmt.Errorf("unknown driver %q", driver)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if driver == "sqlite" {
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(20)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	s := &SQLStorage{db: db, d: d}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLStorage) Close() error { return s.db.Close() }

func (s *SQLStorage) migrate() error {
	ai, ts, eng := s.d.autoInc, s.d.tsType, s.d.engineSuf
	stmts := []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS users (
			id %s,
			username VARCHAR(191) NOT NULL,
			password_hash TEXT NOT NULL,
			role VARCHAR(32) NOT NULL DEFAULT 'user',
			quota_bytes BIGINT NOT NULL DEFAULT %d,
			created_at %s NOT NULL,
			UNIQUE (username)
		)%s`, ai, DefaultQuotaBytes, ts, eng),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS projects (
			id %s,
			user_id BIGINT NOT NULL,
			name VARCHAR(191) NOT NULL,
			domain VARCHAR(191) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			ssl_enabled INTEGER NOT NULL DEFAULT 0,
			path TEXT NOT NULL,
			disk_bytes BIGINT NOT NULL DEFAULT 0,
			created_at %s NOT NULL,
			UNIQUE (domain)
		)%s`, ai, ts, eng),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS metrics (
			id %s,
			project_id BIGINT NOT NULL,
			ts %s NOT NULL,
			disk_bytes BIGINT NOT NULL DEFAULT 0,
			requests_cum BIGINT NOT NULL DEFAULT 0,
			bandwidth_cum BIGINT NOT NULL DEFAULT 0
		)%s`, ai, ts, eng),
		`CREATE INDEX IF NOT EXISTS idx_metrics_project ON metrics (project_id, ts)`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS settings (
			key VARCHAR(191) PRIMARY KEY,
			value TEXT NOT NULL
		)%s`, eng),
	}
	for _, st := range stmts {
		if _, err := s.db.Exec(st); err != nil {
			return err
		}
	}
	return nil
}

func now() string { return time.Now().Format("2006-01-02 15:04:05") }

func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate entry") ||
		strings.Contains(msg, "constraint failed")
}

const userCols = "id, username, password_hash, role, quota_bytes, created_at"

func scanUser(scanner interface{ Scan(...any) error }) (*User, error) {
	var u User
	if err := scanner.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.QuotaBytes, &u.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *SQLStorage) CreateUser(u *User) error {
	if u.CreatedAt == "" {
		u.CreatedAt = now()
	}
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, role, quota_bytes, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.Username, u.PasswordHash, u.Role, u.QuotaBytes, u.CreatedAt)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	if id, err := res.LastInsertId(); err == nil {
		u.ID = id
	}
	return nil
}

func (s *SQLStorage) GetUserByID(id int64) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *SQLStorage) GetUserByUsername(name string) (*User, error) {
	row := s.db.QueryRow(`SELECT `+userCols+` FROM users WHERE username = ?`, name)
	return scanUser(row)
}

const userDetailedCols = "u.id, u.username, u.password_hash, u.role, u.quota_bytes, u.created_at, COALESCE(SUM(p.disk_bytes),0), COUNT(p.id)"

func (s *SQLStorage) ListUsersDetailed() ([]UserDetailed, error) {
	rows, err := s.db.Query(
		`SELECT ` + userDetailedCols + `
		 FROM users u LEFT JOIN projects p ON p.user_id = u.id
		 GROUP BY u.id, u.username, u.password_hash, u.role, u.quota_bytes, u.created_at
		 ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserDetailed
	for rows.Next() {
		var ud UserDetailed
		if err := rows.Scan(&ud.ID, &ud.Username, &ud.PasswordHash, &ud.Role, &ud.QuotaBytes, &ud.CreatedAt, &ud.UsedBytes, &ud.ProjectCount); err != nil {
			return nil, err
		}
		out = append(out, ud)
	}
	return out, rows.Err()
}

func (s *SQLStorage) GetUserDetailed(id int64) (*UserDetailed, error) {
	row := s.db.QueryRow(
		`SELECT `+userDetailedCols+`
		 FROM users u LEFT JOIN projects p ON p.user_id = u.id
		 WHERE u.id = ?
		 GROUP BY u.id, u.username, u.password_hash, u.role, u.quota_bytes, u.created_at`, id)
	var ud UserDetailed
	if err := row.Scan(&ud.ID, &ud.Username, &ud.PasswordHash, &ud.Role, &ud.QuotaBytes, &ud.CreatedAt, &ud.UsedBytes, &ud.ProjectCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ud, nil
}

func (s *SQLStorage) UpdateUserPassword(id int64, hash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *SQLStorage) UpdateUserQuota(id int64, quota int64) error {
	res, err := s.db.Exec(`UPDATE users SET quota_bytes = ? WHERE id = ?`, quota, id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *SQLStorage) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func affected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

const projectCols = "id, user_id, name, domain, status, ssl_enabled, path, disk_bytes, created_at"

func scanProject(row *sql.Row) (*Project, error) {
	var p Project
	if err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Domain, &p.Status, &p.SSLEnabled, &p.Path, &p.DiskBytes, &p.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *SQLStorage) CreateProject(p *Project) error {
	if p.CreatedAt == "" {
		p.CreatedAt = now()
	}
	res, err := s.db.Exec(
		`INSERT INTO projects (user_id, name, domain, status, ssl_enabled, path, disk_bytes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.UserID, p.Name, p.Domain, p.Status, p.SSLEnabled, p.Path, p.DiskBytes, p.CreatedAt)
	if isDuplicate(err) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	if id, err := res.LastInsertId(); err == nil {
		p.ID = id
	}
	return nil
}

func (s *SQLStorage) GetProjectByID(id int64) (*Project, error) {
	return scanProject(s.db.QueryRow(`SELECT `+projectCols+` FROM projects WHERE id = ?`, id))
}

func (s *SQLStorage) GetProjectByDomain(domain string) (*Project, error) {
	return scanProject(s.db.QueryRow(`SELECT `+projectCols+` FROM projects WHERE domain = ?`, domain))
}

func (s *SQLStorage) ListProjectsByUser(userID int64) ([]Project, error) {
	rows, err := s.db.Query(`SELECT `+projectCols+` FROM projects WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Domain, &p.Status, &p.SSLEnabled, &p.Path, &p.DiskBytes, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLStorage) ListAllProjects() ([]Project, error) {
	rows, err := s.db.Query(
		`SELECT p.id, p.user_id, p.name, p.domain, p.status, p.ssl_enabled, p.path, p.disk_bytes, p.created_at, u.username
		 FROM projects p JOIN users u ON u.id = p.user_id ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Domain, &p.Status, &p.SSLEnabled, &p.Path, &p.DiskBytes, &p.CreatedAt, &p.Username); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLStorage) UpdateProject(p *Project) error {
	res, err := s.db.Exec(
		`UPDATE projects SET name = ?, domain = ?, status = ?, ssl_enabled = ?, path = ?, disk_bytes = ? WHERE id = ?`,
		p.Name, p.Domain, p.Status, p.SSLEnabled, p.Path, p.DiskBytes, p.ID)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *SQLStorage) UpdateProjectDisk(id int64, disk int64) error {
	res, err := s.db.Exec(`UPDATE projects SET disk_bytes = ? WHERE id = ?`, disk, id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *SQLStorage) DeleteProject(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM metrics WHERE project_id = ?`, id); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s *SQLStorage) AddMetric(m *Metric) error {
	_, err := s.db.Exec(
		`INSERT INTO metrics (project_id, ts, disk_bytes, requests_cum, bandwidth_cum) VALUES (?, ?, ?, ?, ?)`,
		m.ProjectID, m.TS, m.DiskBytes, m.RequestsCum, m.BandwidthCum)
	return err
}

func (s *SQLStorage) LastMetric(projectID int64) (*Metric, error) {
	row := s.db.QueryRow(
		`SELECT id, project_id, ts, disk_bytes, requests_cum, bandwidth_cum
		 FROM metrics WHERE project_id = ? ORDER BY id DESC LIMIT 1`, projectID)
	var m Metric
	if err := row.Scan(&m.ID, &m.ProjectID, &m.TS, &m.DiskBytes, &m.RequestsCum, &m.BandwidthCum); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

func (s *SQLStorage) ListMetrics(projectID int64, limit int) ([]Metric, error) {
	rows, err := s.db.Query(
		`SELECT id, project_id, ts, disk_bytes, requests_cum, bandwidth_cum
		 FROM (SELECT id, project_id, ts, disk_bytes, requests_cum, bandwidth_cum
		       FROM metrics WHERE project_id = ? ORDER BY id DESC LIMIT ?)
		 ORDER BY id ASC`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Metric
	for rows.Next() {
		var m Metric
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.TS, &m.DiskBytes, &m.RequestsCum, &m.BandwidthCum); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SQLStorage) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

func (s *SQLStorage) SetSetting(key, value string) error {
	if s.d.name == "mysql" {
		_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
			ON DUPLICATE KEY UPDATE value = VALUES(value)`, key, value)
		return err
	}
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}