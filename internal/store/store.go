package store

type Storage interface {
	Close() error

	CreateUser(u *User) error
	GetUserByID(id int64) (*User, error)
	GetUserByUsername(name string) (*User, error)
	GetUserDetailed(id int64) (*UserDetailed, error)
	ListUsersDetailed() ([]UserDetailed, error)
	UpdateUserPassword(id int64, hash string) error
	UpdateUserQuota(id int64, quota int64) error
	CountUsers() (int, error)

	CreateProject(p *Project) error
	GetProjectByID(id int64) (*Project, error)
	GetProjectByDomain(domain string) (*Project, error)
	ListProjectsByUser(userID int64) ([]Project, error)
	ListAllProjects() ([]Project, error)
	UpdateProject(p *Project) error
	UpdateProjectDisk(id int64, disk int64) error
	DeleteProject(id int64) error

	AddMetric(m *Metric) error
	LastMetric(projectID int64) (*Metric, error)
	ListMetrics(projectID int64, limit int) ([]Metric, error)

	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
}