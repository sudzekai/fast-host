package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxMetricsPerProject = 500

type jsonList[T any] struct {
	NextID int64 `json:"next_id"`
	Items  []T   `json:"items"`
}

type jsonUserRecord struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
	QuotaBytes   int64  `json:"quota_bytes"`
	CreatedAt    string `json:"created_at"`
}

type jsonProjectRecord struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Name       string `json:"name"`
	Domain     string `json:"domain"`
	Status     string `json:"status"`
	SSLEnabled bool   `json:"ssl_enabled"`
	Path       string `json:"path"`
	DiskBytes  int64  `json:"disk_bytes"`
	CreatedAt  string `json:"created_at"`
}

type jsonMetricRecord struct {
	ID           int64  `json:"id"`
	ProjectID    int64  `json:"project_id"`
	TS           string `json:"ts"`
	DiskBytes    int64  `json:"disk_bytes"`
	RequestsCum  int64  `json:"requests_cum"`
	BandwidthCum int64  `json:"bandwidth_cum"`
}

type jsonStore struct {
	mu sync.Mutex

	dir string

	users    []jsonUserRecord
	nextUID  int64
	userByID map[int64]*jsonUserRecord
	userName map[string]*jsonUserRecord

	projects    []jsonProjectRecord
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
	var users jsonList[jsonUserRecord]
	if err := readJSONFile(s.dataPath("users.json"), &users); err != nil {
		return err
	}
	s.users, s.nextUID = users.Items, users.NextID
	for i := range s.users {
		rec := &s.users[i]
		s.userByID[rec.ID] = rec
		s.userName[rec.Username] = rec
	}

	var projects jsonList[jsonProjectRecord]
	if err := readJSONFile(s.dataPath("projects.json"), &projects); err != nil {
		return err
	}
	s.projects, s.nextPID = projects.Items, projects.NextID
	for i := range s.projects {
		rec := &s.projects[i]
		s.projectByID[rec.ID] = rec
		s.projectDom[rec.Domain] = rec
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
	return s.writeFile(s.dataPath("users.json"), jsonList[jsonUserRecord]{NextID: s.nextUID, Items: s.users})
}

func (s *jsonStore) saveProjects() error {
	return s.writeFile(s.dataPath("projects.json"), jsonList[jsonProjectRecord]{NextID: s.nextPID, Items: s.projects})
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
		ID: rec.ID, Username: rec.Username, PasswordHash: rec.PasswordHash,
		Role: rec.Role, QuotaBytes: rec.QuotaBytes, CreatedAt: rec.CreatedAt,
	}
}

func toProject(rec *jsonProjectRecord) *Project {
	return &Project{
		ID: rec.ID, UserID: rec.UserID, Name: rec.Name, Domain: rec.Domain,
		Status: rec.Status, SSLEnabled: rec.SSLEnabled, Path: rec.Path,
		DiskBytes: rec.DiskBytes, CreatedAt: rec.CreatedAt,
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
		u.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	rec := jsonUserRecord{
		ID: id, Username: u.Username, PasswordHash: u.PasswordHash,
		Role: u.Role, QuotaBytes: u.QuotaBytes, CreatedAt: u.CreatedAt,
	}
	s.users = append(s.users, rec)
	s.userByID[id] = &s.users[len(s.users)-1]
	s.userName[u.Username] = &s.users[len(s.users)-1]
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

func (s *jsonStore) ListUsersDetailed() ([]UserDetailed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]UserDetailed, 0, len(s.users))
	for i := range s.users {
		u := toUser(&s.users[i])
		var used int64
		count := 0
		for j := range s.projects {
			if s.projects[j].UserID == u.ID {
				used += s.projects[j].DiskBytes
				count++
			}
		}
		out = append(out, UserDetailed{User: *u, UsedBytes: used, ProjectCount: count})
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
	var used int64
	count := 0
	for j := range s.projects {
		if s.projects[j].UserID == id {
			used += s.projects[j].DiskBytes
			count++
		}
	}
	u := toUser(rec)
	return &UserDetailed{User: *u, UsedBytes: used, ProjectCount: count}, nil
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
		p.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	rec := jsonProjectRecord{
		ID: id, UserID: p.UserID, Name: p.Name, Domain: p.Domain,
		Status: p.Status, SSLEnabled: p.SSLEnabled, Path: p.Path,
		DiskBytes: p.DiskBytes, CreatedAt: p.CreatedAt,
	}
	s.projects = append(s.projects, rec)
	s.projectByID[id] = &s.projects[len(s.projects)-1]
	s.projectDom[p.Domain] = &s.projects[len(s.projects)-1]
	p.ID = id
	return s.saveProjects()
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
	for i := range s.projects {
		if s.projects[i].UserID == userID {
			out = append(out, *toProject(&s.projects[i]))
		}
	}
	return out, nil
}

func (s *jsonStore) ListAllProjects() ([]Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Project, 0, len(s.projects))
	for i := range s.projects {
		p := toProject(&s.projects[i])
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
		delete(s.projectDom, rec.Domain)
		s.projectDom[p.Domain] = rec
	}
	rec.Name, rec.Domain, rec.Status = p.Name, p.Domain, p.Status
	rec.SSLEnabled, rec.Path, rec.DiskBytes = p.SSLEnabled, p.Path, p.DiskBytes
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
	keep := s.projects[:0]
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

func (s *jsonStore) UsersFile() string   { return s.dataPath("users.json") }
func (s *jsonStore) ProjectsFile() string { return s.dataPath("projects.json") }
func (s *jsonStore) Dir() string          { return s.dir }