package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const maxMetricsPerProject = 500

type jsonList[T any] struct {
	NextID int64 `json:"next_id"`
	Items  []T   `json:"items"`
}

type jsonUserRecord struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	Email        string `json:"email"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
	QuotaBytes   int64  `json:"quota_bytes"`
	CreatedAt    string `json:"created_at"`
}

// Disabled вместо Enabled: у записей, сохранённых до появления поля,
// значение по умолчанию (false) означает «проект включён».
type jsonProjectRecord struct {
	ID           int64  `json:"id"`
	UserID       int64  `json:"user_id"`
	Name         string `json:"name"`
	Domain       string `json:"domain"`
	Status       string `json:"status"`
	Disabled     bool   `json:"disabled,omitempty"`
	SSLEnabled   bool   `json:"ssl_enabled"`
	SSLIssuedAt  string `json:"ssl_issued_at,omitempty"`
	SSLExpiresAt string `json:"ssl_expires_at,omitempty"`
	Path         string `json:"path"`
	DiskBytes    int64  `json:"disk_bytes"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

type jsonMetricRecord struct {
	ID           int64  `json:"id"`
	ProjectID    int64  `json:"project_id"`
	TS           string `json:"ts"`
	DiskBytes    int64  `json:"disk_bytes"`
	RequestsCum  int64  `json:"requests_cum"`
	BandwidthCum int64  `json:"bandwidth_cum"`
}

// Записи хранятся по указателям: индексы-мапы ссылаются на те же объекты,
// поэтому append (перевыделение массива) и удаление их не инвалидируют.
type jsonStore struct {
	mu sync.Mutex

	dir string

	users    []*jsonUserRecord
	nextUID  int64
	userByID map[int64]*jsonUserRecord
	userName map[string]*jsonUserRecord

	projects    []*jsonProjectRecord
	nextPID     int64
	projectByID map[int64]*jsonProjectRecord
	projectDom  map[string]*jsonProjectRecord

	metrics   map[int64][]jsonMetricRecord
	metricsID map[int64]int64

	settings map[string]string
}

var _ Storage = (*jsonStore)(nil)

func NewJSONStorage(dir string) (*jsonStore, error) {
	if dir == "" {
		dir = "data/fasthost-json"
	}
	s := &jsonStore{
		dir:         dir,
		userByID:    map[int64]*jsonUserRecord{},
		userName:    map[string]*jsonUserRecord{},
		projectByID: map[int64]*jsonProjectRecord{},
		projectDom:  map[string]*jsonProjectRecord{},
		metrics:     map[int64][]jsonMetricRecord{},
		metricsID:   map[int64]int64{},
		settings:    map[string]string{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *jsonStore) dataPath(name string) string {
	return filepath.Join(s.dir, name)
}

func (s *jsonStore) metricsPath(pid int64) string {
	return filepath.Join(s.dir, "metrics", fmt.Sprintf("%d.json", pid))
}

func (s *jsonStore) load() error {
	if err := os.MkdirAll(filepath.Join(s.dir, "metrics"), 0o755); err != nil {
		return err
	}
	var users jsonList[*jsonUserRecord]
	if err := readJSONFile(s.dataPath("users.json"), &users); err != nil {
		return err
	}
	s.users, s.nextUID = users.Items, users.NextID
	for _, rec := range s.users {
		s.userByID[rec.ID] = rec
		s.userName[rec.Username] = rec
		if rec.ID >= s.nextUID {
			s.nextUID = rec.ID + 1
		}
	}

	var projects jsonList[*jsonProjectRecord]
	if err := readJSONFile(s.dataPath("projects.json"), &projects); err != nil {
		return err
	}
	s.projects, s.nextPID = projects.Items, projects.NextID
	for _, rec := range s.projects {
		s.projectByID[rec.ID] = rec
		s.projectDom[rec.Domain] = rec
		if rec.ID >= s.nextPID {
			s.nextPID = rec.ID + 1
		}
	}

	var settings struct {
		Settings map[string]string `json:"settings"`
	}
	if err := readJSONFile(s.dataPath("settings.json"), &settings); err != nil {
		return err
	}
	s.settings = settings.Settings
	if s.settings == nil {
		s.settings = map[string]string{}
	}
	if s.nextUID == 0 {
		s.nextUID = 1
	}
	if s.nextPID == 0 {
		s.nextPID = 1
	}
	return nil
}

func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *jsonStore) writeFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *jsonStore) saveUsers() error {
	return s.writeFile(s.dataPath("users.json"), jsonList[*jsonUserRecord]{NextID: s.nextUID, Items: s.users})
}

func (s *jsonStore) saveProjects() error {
	return s.writeFile(s.dataPath("projects.json"), jsonList[*jsonProjectRecord]{NextID: s.nextPID, Items: s.projects})
}

func (s *jsonStore) saveSettings() error {
	return s.writeFile(s.dataPath("settings.json"), struct {
		Settings map[string]string `json:"settings"`
	}{Settings: s.settings})
}

func (s *jsonStore) saveMetrics(pid int64) error {
	items := s.metrics[pid]
	return s.writeFile(s.metricsPath(pid), jsonList[jsonMetricRecord]{NextID: s.metricsID[pid], Items: items})
}

func (s *jsonStore) Close() error { return nil }

func toUser(rec *jsonUserRecord) *User {
	return &User{
		ID: rec.ID, Username: rec.Username, Email: rec.Email, PasswordHash: rec.PasswordHash,
		Role: rec.Role, QuotaBytes: rec.QuotaBytes, CreatedAt: rec.CreatedAt,
	}
}

func toProject(rec *jsonProjectRecord) *Project {
	return &Project{
		ID: rec.ID, UserID: rec.UserID, Name: rec.Name, Domain: rec.Domain,
		Status: rec.Status, Enabled: !rec.Disabled,
		SSLEnabled: rec.SSLEnabled, SSLIssuedAt: rec.SSLIssuedAt, SSLExpiresAt: rec.SSLExpiresAt,
		Path: rec.Path, DiskBytes: rec.DiskBytes, CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
	}
}

func toMetric(rec *jsonMetricRecord) *Metric {
	return &Metric{
		ID: rec.ID, ProjectID: rec.ProjectID, TS: rec.TS,
		DiskBytes: rec.DiskBytes, RequestsCum: rec.RequestsCum, BandwidthCum: rec.BandwidthCum,
	}
}

func (s *jsonStore) CreateUser(u *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.userName[u.Username]; ok {
		return ErrDuplicate
	}
	id := s.nextUID
	s.nextUID++
	if u.CreatedAt == "" {
		u.CreatedAt = Now()
	}
	rec := &jsonUserRecord{
		ID: id, Username: u.Username, Email: u.Email, PasswordHash: u.PasswordHash,
		Role: u.Role, QuotaBytes: u.QuotaBytes, CreatedAt: u.CreatedAt,
	}
	s.users = append(s.users, rec)
	s.userByID[id] = rec
	s.userName[u.Username] = rec
	u.ID = id
	return s.saveUsers()
}

func (s *jsonStore) GetUserByID(id int64) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.userByID[id]
	if !ok {
		return nil, ErrNotFound
	}
	return toUser(rec), nil
}

func (s *jsonStore) GetUserByUsername(name string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.userName[name]
	if !ok {
		return nil, ErrNotFound
	}
	return toUser(rec), nil
}

func (s *jsonStore) usageLocked(userID int64) (used int64, count int) {
	for _, p := range s.projects {
		if p.UserID == userID {
			used += p.DiskBytes
			count++
		}
	}
	return used, count
}

func (s *jsonStore) ListUsersDetailed() ([]UserDetailed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]UserDetailed, 0, len(s.users))
	for _, rec := range s.users {
		used, count := s.usageLocked(rec.ID)
		out = append(out, UserDetailed{User: *toUser(rec), UsedBytes: used, ProjectCount: count})
	}
	return out, nil
}

func (s *jsonStore) GetUserDetailed(id int64) (*UserDetailed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.userByID[id]
	if !ok {
		return nil, ErrNotFound
	}
	used, count := s.usageLocked(id)
	return &UserDetailed{User: *toUser(rec), UsedBytes: used, ProjectCount: count}, nil
}

func (s *jsonStore) UpdateUserPassword(id int64, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.userByID[id]
	if !ok {
		return ErrNotFound
	}
	rec.PasswordHash = hash
	return s.saveUsers()
}

func (s *jsonStore) UpdateUserQuota(id int64, quota int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.userByID[id]
	if !ok {
		return ErrNotFound
	}
	rec.QuotaBytes = quota
	return s.saveUsers()
}

func (s *jsonStore) CountUsers() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users), nil
}

func (s *jsonStore) CreateProject(p *Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projectDom[p.Domain]; ok {
		return ErrDuplicate
	}
	id := s.nextPID
	s.nextPID++
	if p.CreatedAt == "" {
		p.CreatedAt = Now()
	}
	rec := &jsonProjectRecord{ID: id, UserID: p.UserID, CreatedAt: p.CreatedAt}
	fillProjectRecord(rec, p)
	s.projects = append(s.projects, rec)
	s.projectByID[id] = rec
	s.projectDom[p.Domain] = rec
	p.ID = id
	return s.saveProjects()
}

func fillProjectRecord(rec *jsonProjectRecord, p *Project) {
	rec.Name, rec.Domain, rec.Status = p.Name, p.Domain, p.Status
	rec.Disabled = !p.Enabled
	rec.SSLEnabled, rec.SSLIssuedAt, rec.SSLExpiresAt = p.SSLEnabled, p.SSLIssuedAt, p.SSLExpiresAt
	rec.Path, rec.DiskBytes, rec.UpdatedAt = p.Path, p.DiskBytes, p.UpdatedAt
}

func (s *jsonStore) GetProjectByID(id int64) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.projectByID[id]
	if !ok {
		return nil, ErrNotFound
	}
	return toProject(rec), nil
}

func (s *jsonStore) GetProjectByDomain(domain string) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.projectDom[domain]
	if !ok {
		return nil, ErrNotFound
	}
	return toProject(rec), nil
}

func (s *jsonStore) ListProjectsByUser(userID int64) ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Project
	for _, rec := range s.projects {
		if rec.UserID == userID {
			out = append(out, *toProject(rec))
		}
	}
	return out, nil
}

func (s *jsonStore) ListAllProjects() ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Project, 0, len(s.projects))
	for _, rec := range s.projects {
		p := toProject(rec)
		if owner, ok := s.userByID[p.UserID]; ok {
			p.Username = owner.Username
		}
		out = append(out, *p)
	}
	return out, nil
}

func (s *jsonStore) UpdateProject(p *Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.projectByID[p.ID]
	if !ok {
		return ErrNotFound
	}
	if rec.Domain != p.Domain {
		if other, taken := s.projectDom[p.Domain]; taken && other != rec {
			return ErrDuplicate
		}
		delete(s.projectDom, rec.Domain)
		s.projectDom[p.Domain] = rec
	}
	fillProjectRecord(rec, p)
	return s.saveProjects()
}

func (s *jsonStore) UpdateProjectDisk(id int64, disk int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.projectByID[id]
	if !ok {
		return ErrNotFound
	}
	rec.DiskBytes = disk
	return s.saveProjects()
}

func (s *jsonStore) DeleteProject(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.projectByID[id]
	if !ok {
		return ErrNotFound
	}
	_ = os.Remove(s.metricsPath(id))
	delete(s.metrics, id)
	delete(s.metricsID, id)
	delete(s.projectDom, rec.Domain)
	delete(s.projectByID, id)
	keep := make([]*jsonProjectRecord, 0, len(s.projects))
	for _, p := range s.projects {
		if p.ID != id {
			keep = append(keep, p)
		}
	}
	s.projects = keep
	return s.saveProjects()
}

func (s *jsonStore) loadMetricsLocked(pid int64) {
	if _, ok := s.metrics[pid]; ok {
		return
	}
	var list jsonList[jsonMetricRecord]
	if err := readJSONFile(s.metricsPath(pid), &list); err == nil {
		s.metrics[pid] = list.Items
		s.metricsID[pid] = list.NextID
	}
	if s.metricsID[pid] == 0 {
		s.metricsID[pid] = 1
	}
}

func (s *jsonStore) AddMetric(m *Metric) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projectByID[m.ProjectID]; !ok {
		// проект удалён, пока сборщик метрик его обрабатывал
		return ErrNotFound
	}
	s.loadMetricsLocked(m.ProjectID)
	id := s.metricsID[m.ProjectID]
	s.metricsID[m.ProjectID] = id + 1
	rec := jsonMetricRecord{
		ID: id, ProjectID: m.ProjectID, TS: m.TS,
		DiskBytes: m.DiskBytes, RequestsCum: m.RequestsCum, BandwidthCum: m.BandwidthCum,
	}
	s.metrics[m.ProjectID] = append(s.metrics[m.ProjectID], rec)
	if len(s.metrics[m.ProjectID]) > maxMetricsPerProject {
		s.metrics[m.ProjectID] = s.metrics[m.ProjectID][len(s.metrics[m.ProjectID])-maxMetricsPerProject:]
	}
	return s.saveMetrics(m.ProjectID)
}

func (s *jsonStore) LastMetric(projectID int64) (*Metric, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadMetricsLocked(projectID)
	items := s.metrics[projectID]
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return toMetric(&items[len(items)-1]), nil
}

func (s *jsonStore) ListMetrics(projectID int64, limit int) ([]Metric, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadMetricsLocked(projectID)
	items := s.metrics[projectID]
	if len(items) == 0 {
		return []Metric{}, nil
	}
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	start := len(items) - limit
	out := make([]Metric, 0, limit)
	for i := start; i < len(items); i++ {
		out = append(out, *toMetric(&items[i]))
	}
	return out, nil
}

func (s *jsonStore) GetSetting(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.settings[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *jsonStore) SetSetting(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[key] = value
	return s.saveSettings()
}

func (s *jsonStore) DeleteSetting(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.settings[key]; !ok {
		return nil
	}
	delete(s.settings, key)
	return s.saveSettings()
}

func (s *jsonStore) UsersFile() string    { return s.dataPath("users.json") }
func (s *jsonStore) ProjectsFile() string { return s.dataPath("projects.json") }
func (s *jsonStore) Dir() string          { return s.dir }
