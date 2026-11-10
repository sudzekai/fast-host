package nginx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

var domainRe = regexp.MustCompile(`^(?i)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

func ValidateDomain(d string) error {
	if d == "localhost" {
		return nil
	}
	if !domainRe.MatchString(d) {
		return fmt.Errorf("некорректный домен: %s", d)
	}
	if len(d) > 253 {
		return fmt.Errorf("домен слишком длинный")
	}
	return nil
}

// Aliases — имена, на которые отвечает сайт: домен и его www-вариант.
func Aliases(domain string) []string {
	if strings.HasPrefix(domain, "www.") {
		return []string{domain, strings.TrimPrefix(domain, "www.")}
	}
	return []string{domain, "www." + domain}
}

type Options struct {
	Bin       string // исполняемый файл nginx
	MainConf  string // свой главный конфиг (управляемый режим); пусто — системный nginx
	SitesDir  string // каталог, который главный конфиг подключает через include *.conf
	ReloadCmd string // своя команда перезагрузки (например "sudo systemctl reload nginx")
	HTTPPort  int
	HTTPSPort int
	// DefaultServer — создавать заглушку default_server, чтобы запросы к выключенным
	// и неизвестным доменам не попадали на чужой сайт.
	DefaultServer bool

	// PHPFPM — адрес php-fpm для fastcgi_pass (например "unix:/run/php/php-fpm.sock"
	// или "127.0.0.1:9000"). Пусто — PHP не подключается.
	PHPFPM string

	AcmeDir      string // webroot для HTTP-01 проверки Let's Encrypt
	SSLDir       string // самоподписанные сертификаты и локальный CA
	CertbotBin   string
	CertbotDir   string // --config-dir certbot (без root)
	CertbotEmail string
	CertbotStage bool   // тестовый сервер Let's Encrypt (--test-cert): без лимитов, но браузеры не доверяют
	SSLProvider  string // auto | letsencrypt | selfsigned
}

type Manager struct {
	opt Options
	mu  sync.Mutex
}

func NewManager(opt Options) *Manager {
	if opt.Bin == "" {
		opt.Bin = "nginx"
	}
	if opt.CertbotBin == "" {
		opt.CertbotBin = "certbot"
	}
	if opt.HTTPPort == 0 {
		opt.HTTPPort = 80
	}
	if opt.HTTPSPort == 0 {
		opt.HTTPSPort = 443
	}
	if opt.SSLProvider == "" {
		opt.SSLProvider = "auto"
	}
	return &Manager{opt: opt}
}

func (m *Manager) Options() Options { return m.opt }

// Site — всё, что нужно для генерации server-блоков одного проекта.
type Site struct {
	Provider string
	PHPFPM   string
	Domain   string
	Root     string // каталог с файлами сайта
	LogDir   string // access.log / error.log
	SSL      bool
	CertFile string
	KeyFile  string
}

var siteTpl = template.Must(template.New("site").Parse(`server {
    listen {{.HTTPPort}};
    listen [::]:{{.HTTPPort}};
    server_name {{.ServerNames}};
{{- if .SSL}}

    access_log {{.LogDir}}/access.log;
    error_log {{.LogDir}}/error.log;

    location ^~ /.well-known/acme-challenge/ {
        root {{.AcmeDir}};
        default_type text/plain;
    }

    location / {
        return 301 https://$host{{.HTTPSSuffix}}$request_uri;
    }
}

server {
    listen {{.HTTPSPort}} ssl;
    listen [::]:{{.HTTPSPort}} ssl;
    server_name {{.ServerNames}};

    ssl_certificate {{.CertFile}};
    ssl_certificate_key {{.KeyFile}};
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_timeout 1d;
    add_header Strict-Transport-Security "max-age=31536000" always;
{{- else}}

    location ^~ /.well-known/acme-challenge/ {
        root {{.AcmeDir}};
        default_type text/plain;
    }
{{- end}}

    root {{.Root}};
{{- if .PHP}}
    index index.php index.html index.htm;
{{- else}}
    index index.html index.htm;
{{- end}}

    access_log {{.LogDir}}/access.log;
    error_log {{.LogDir}}/error.log;

    location ~ /\.(?!well-known/) {
        return 404;
    }
{{- if .PHP}}

    location ~ \.php$ {
        try_files $uri =404;
        fastcgi_split_path_info ^(.+\.php)(/.+)$;
        fastcgi_pass {{.PHPFPM}};
        fastcgi_index index.php;

        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        fastcgi_param PATH_INFO $fastcgi_path_info;
        fastcgi_param QUERY_STRING $query_string;
        fastcgi_param REQUEST_METHOD $request_method;
        fastcgi_param CONTENT_TYPE $content_type;
        fastcgi_param CONTENT_LENGTH $content_length;
        fastcgi_param SCRIPT_NAME $fastcgi_script_name;
        fastcgi_param REQUEST_URI $request_uri;
        fastcgi_param DOCUMENT_URI $document_uri;
        fastcgi_param DOCUMENT_ROOT $document_root;
        fastcgi_param SERVER_PROTOCOL $server_protocol;
        fastcgi_param REQUEST_SCHEME $scheme;
        fastcgi_param HTTPS $https if_not_empty;
        fastcgi_param GATEWAY_INTERFACE CGI/1.1;
        fastcgi_param SERVER_SOFTWARE nginx/$nginx_version;
        fastcgi_param REMOTE_ADDR $remote_addr;
        fastcgi_param REMOTE_PORT $remote_port;
        fastcgi_param SERVER_ADDR $server_addr;
        fastcgi_param SERVER_PORT $server_port;
        fastcgi_param SERVER_NAME $server_name;
        fastcgi_param REDIRECT_STATUS 200;
    }
{{- else}}

    location ~ \.php(?:/|$) {
        return 404;
    }
{{- end}}

    location / {
{{- if .PHP}}
        try_files $uri $uri/ /index.html /index.php?$query_string;
{{- else}}
        try_files $uri $uri/ /index.html;
{{- end}}
    }

    location ~* \.(css|js|png|jpg|jpeg|gif|svg|ico|webp|avif|woff2?|ttf)$ {
        expires 7d;
        add_header Cache-Control "public, max-age=604800";
    }
}
`))

var defaultTpl = template.Must(template.New("default").Parse(`server {
    listen {{.HTTPPort}} default_server;
    listen [::]:{{.HTTPPort}} default_server;
    server_name _;
    return 444;
}

server {
    listen {{.HTTPSPort}} ssl default_server;
    listen [::]:{{.HTTPSPort}} ssl default_server;
    server_name _;
    ssl_reject_handshake on;
}
`))

var mainTpl = template.Must(template.New("main").Parse(`{{if .User}}user {{.User}};
{{end}}worker_processes auto;
pid {{.Dir}}/nginx.pid;
error_log {{.Dir}}/error.log;

events {
    worker_connections 1024;
}

http {
    types {
        text/html html htm;
        text/css css;
        text/plain txt;
        text/xml xml;
        application/javascript js mjs;
        application/json json;
        application/wasm wasm;
        application/pdf pdf;
        application/zip zip;
        image/png png;
        image/jpeg jpg jpeg;
        image/gif gif;
        image/svg+xml svg svgz;
        image/webp webp;
        image/avif avif;
        image/x-icon ico;
        font/woff woff;
        font/woff2 woff2;
        font/ttf ttf;
        audio/mpeg mp3;
        video/mp4 mp4;
        video/webm webm;
    }
    default_type application/octet-stream;

    sendfile on;
    keepalive_timeout 65;
    server_tokens off;

    client_body_temp_path {{.Dir}}/temp/client_body;
    proxy_temp_path {{.Dir}}/temp/proxy;
    fastcgi_temp_path {{.Dir}}/temp/fastcgi;
    uwsgi_temp_path {{.Dir}}/temp/uwsgi;
    scgi_temp_path {{.Dir}}/temp/scgi;

    access_log off;

    include {{.SitesDir}}/*.conf;
}
`))

func (m *Manager) Render(s Site) string {
	php := m.opt.PHPFPM
	if s.Provider == "nginx" {
		php = ""
	}
	if s.Provider == "php-fpm" {
		php = s.PHPFPM
	}
	httpsSuffix := ""
	if m.opt.HTTPSPort != 443 {
		httpsSuffix = ":" + strconv.Itoa(m.opt.HTTPSPort)
	}
	var b bytes.Buffer
	_ = siteTpl.Execute(&b, map[string]any{
		"HTTPPort":    m.opt.HTTPPort,
		"HTTPSPort":   m.opt.HTTPSPort,
		"HTTPSSuffix": httpsSuffix,
		"ServerNames": strings.Join(Aliases(s.Domain), " "),
		"Root":        filepath.ToSlash(s.Root),
		"LogDir":      filepath.ToSlash(s.LogDir),
		"AcmeDir":     filepath.ToSlash(m.opt.AcmeDir),
		"SSL":         s.SSL,
		"CertFile":    filepath.ToSlash(s.CertFile),
		"KeyFile":     filepath.ToSlash(s.KeyFile),
		"PHP":         php != "",
		"PHPFPM":      php,
	})
	return b.String()
}

func (m *Manager) sitePath(domain string) string {
	return filepath.Join(m.opt.SitesDir, domain+".conf")
}

// Init готовит каталоги, заглушку default_server и (в управляемом режиме) главный конфиг.
func (m *Manager) Init() error {
	for _, d := range []string{m.opt.SitesDir, m.opt.AcmeDir, m.opt.SSLDir} {
		if d == "" {
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	defPath := filepath.Join(m.opt.SitesDir, "00-default.conf")
	if m.opt.DefaultServer {
		var b bytes.Buffer
		_ = defaultTpl.Execute(&b, m.opt)
		if err := os.WriteFile(defPath, b.Bytes(), 0o644); err != nil {
			return err
		}
	} else {
		_ = os.Remove(defPath)
	}
	if m.opt.MainConf == "" {
		return nil
	}
	dir := filepath.Dir(m.opt.MainConf)
	for _, sub := range []string{"client_body", "proxy", "fastcgi", "uwsgi", "scgi"} {
		if err := os.MkdirAll(filepath.Join(dir, "temp", sub), 0o755); err != nil {
			return err
		}
	}
	var b bytes.Buffer
	workerUser := ""
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		if _, err := user.Lookup("www-data"); err == nil {
			workerUser = "www-data"
		}
	}
	_ = mainTpl.Execute(&b, map[string]string{"Dir": filepath.ToSlash(dir), "SitesDir": filepath.ToSlash(m.opt.SitesDir), "User": workerUser})
	return os.WriteFile(m.opt.MainConf, b.Bytes(), 0o644)
}

func (m *Manager) nginxArgs(args ...string) []string {
	if m.opt.MainConf != "" {
		args = append(args, "-c", m.opt.MainConf)
	}
	return args
}

func run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		var ee *exec.Error
		if errors.As(err, &ee) {
			return "", fmt.Errorf("%s не найден: установите его или укажите путь флагом", name)
		}
		return string(out), fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (m *Manager) Test() error {
	_, err := run(m.opt.Bin, m.nginxArgs("-t", "-q")...)
	return err
}

var systemPidFiles = []string{"/run/nginx.pid", "/var/run/nginx.pid", "/opt/homebrew/var/run/nginx.pid", "/usr/local/var/run/nginx.pid"}

// masterPID — pid главного процесса nginx (0, если не найден или не работает).
func (m *Manager) masterPID() int {
	files := systemPidFiles
	if m.opt.MainConf != "" {
		files = []string{filepath.Join(filepath.Dir(m.opt.MainConf), "nginx.pid")}
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err == nil && pid > 0 && processAlive(pid) {
			return pid
		}
	}
	return 0
}

func (m *Manager) running() bool {
	return m.opt.MainConf == "" || m.masterPID() != 0
}

func workers(master int) string {
	out, _ := exec.Command("pgrep", "-P", strconv.Itoa(master)).Output()
	return strings.TrimSpace(string(out))
}

// waitReload ждёт, пока master запустит воркеров с новой конфигурацией: сигнал reload
// асинхронный, и без ожидания запрос сразу после API-вызова попал бы на старый конфиг.
func waitReload(master int, before string) {
	if master == 0 || before == "" {
		time.Sleep(300 * time.Millisecond)
		return
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if w := workers(master); w != "" && !sharesAny(w, before) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func sharesAny(now, before string) bool {
	old := map[string]bool{}
	for _, p := range strings.Fields(before) {
		old[p] = true
	}
	for _, p := range strings.Fields(now) {
		if old[p] {
			return true
		}
	}
	return false
}

// Reload проверяет конфигурацию и применяет её; в управляемом режиме запускает nginx, если он не работает.
func (m *Manager) Reload() error {
	if err := m.Test(); err != nil {
		return err
	}
	if !m.running() {
		_, err := run(m.opt.Bin, m.nginxArgs()...)
		return err
	}
	master := m.masterPID()
	before := ""
	if master != 0 {
		before = workers(master)
	}
	var err error
	if m.opt.ReloadCmd != "" {
		f := strings.Fields(m.opt.ReloadCmd)
		_, err = run(f[0], f[1:]...)
	} else {
		_, err = run(m.opt.Bin, m.nginxArgs("-s", "reload")...)
	}
	if err == nil {
		waitReload(master, before)
	}
	return err
}

// Stop останавливает nginx, запущенный в управляемом режиме.
func (m *Manager) Stop() error {
	if m.opt.MainConf == "" || !m.running() {
		return nil
	}
	_, err := run(m.opt.Bin, m.nginxArgs("-s", "quit")...)
	return err
}

// Write кладёт (enabled) или убирает конфиг сайта, не перезагружая nginx.
func (m *Manager) Write(s Site, enabled bool) error {
	path := m.sitePath(s.Domain)
	if !enabled {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(s.LogDir, 0o755); err != nil {
		return err
	}
	_ = os.Chmod(s.LogDir, 0o777)
	return os.WriteFile(path, []byte(m.Render(s)), 0o644)
}

// Apply записывает конфиг сайта и перезагружает nginx. Если новая конфигурация не
// проходит nginx -t, возвращается прежний файл, чтобы не сломать остальные сайты.
func (m *Manager) Apply(s Site, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	path := m.sitePath(s.Domain)
	prev, prevErr := os.ReadFile(path)
	if err := m.Write(s, enabled); err != nil {
		return err
	}
	if err := m.Reload(); err != nil {
		if prevErr == nil {
			_ = os.WriteFile(path, prev, 0o644)
		} else {
			_ = os.Remove(path)
		}
		return err
	}
	return nil
}

// ReloadAll применяет конфиги, записанные через Write.
func (m *Manager) ReloadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Reload()
}

// Remove убирает сайт из nginx и удаляет его сертификаты. Конфиг удаляется даже при
// ошибке перезагрузки: оставлять его нельзя, он ссылается на удаляемый каталог.
func (m *Manager) Remove(domain string) error {
	m.mu.Lock()
	err := m.Write(Site{Domain: domain}, false)
	if err == nil {
		err = m.Reload()
	}
	m.mu.Unlock()
	_ = os.RemoveAll(filepath.Join(m.opt.SSLDir, domain))
	if m.opt.CertbotDir != "" {
		if _, statErr := os.Stat(filepath.Join(m.opt.CertbotDir, "renewal", domain+".conf")); statErr == nil {
			_, _ = run(m.opt.CertbotBin, append([]string{"delete", "--cert-name", domain, "--non-interactive"}, m.certbotDirs()...)...)
		}
	}
	return err
}
