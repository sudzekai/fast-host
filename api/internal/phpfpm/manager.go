// Package phpfpm manages isolated Debian PHP-FPM pools. Administrative operations
// require FastHost to run as root; PHP workers always run as project users.
package phpfpm

import (
	"context"
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
	"time"

	"hosting/internal/store"
)

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
var envRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}_DIR$`)
var versionRE = regexp.MustCompile(`^(?:8\.(?:[2-9]|[1-9][0-9]+)|9\.[0-9]+)$`)

type Manager struct {
	DataDir, Version string
	mu               sync.Mutex
}

func ValidateDirectory(d store.DataDirectory) error {
	if !nameRE.MatchString(d.Name) || !envRE.MatchString(d.Env) {
		return errors.New("имя каталога: a-z, 0-9, _-, до 32 символов; переменная: заглавные латинские буквы, цифры, _, с окончанием _DIR")
	}
	return nil
}

func (m *Manager) paths(p *store.Project) (config, socket string) {
	key := store.ProjectKey(p)
	return "/etc/php/" + m.Version + "/fpm/pool.d/" + key + ".conf", "/run/php/" + key + ".sock"
}

func (m *Manager) Socket(p *store.Project) string { _, s := m.paths(p); return "unix:" + s }

func command(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Manager) check() error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return errors.New("для управления отдельными PHP-FPM пулами запустите API от root на Linux")
	}
	if !versionRE.MatchString(m.Version) {
		return errors.New("требуется PHP-FPM 8.2+; настройте --php-version")
	}
	if _, err := os.Stat("/etc/php/" + m.Version + "/fpm/pool.d"); err != nil {
		return fmt.Errorf("установите php%s-fpm и php%s-sqlite3: %w", m.Version, m.Version, err)
	}
	if err := command("php"+m.Version, "-r", `if (PHP_VERSION_ID < 80200 || !extension_loaded('pdo_sqlite') || !in_array('argon2id', password_algos(), true)) {fwrite(STDERR, 'PHP 8.2+, pdo_sqlite and Argon2id required'); exit(1);} $h=password_hash('probe', PASSWORD_ARGON2ID); if (!password_verify('probe', $h)) exit(1);`); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := exec.CommandContext(ctx, "php-fpm"+m.Version, "-i").CombinedOutput()
	if err != nil {
		return fmt.Errorf("проверка PHP-FPM: %w", err)
	}
	for _, line := range strings.Split(string(info), "\n") {
		if strings.HasPrefix(line, "PDO drivers =>") && strings.Contains(line, "sqlite") {
			return nil
		}
	}
	return errors.New("pdo_sqlite должен быть включён в PHP-FPM, а не только в PHP CLI")
}

func quote(s string) string {
	s = filepath.ToSlash(s)
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func Render(p *store.Project, root, socket string) (string, error) {
	// FPM expands $variables even in quoted INI values.
	if strings.ContainsAny(root+p.Path+socket, "$\r\n\x00\";") {
		return "", errors.New("путь содержит недопустимые для PHP-FPM символы")
	}
	key := store.ProjectKey(p)
	var b strings.Builder
	fmt.Fprintf(&b, "[%s]\nuser = %s\ngroup = %s\nlisten = %s\nlisten.owner = www-data\nlisten.group = www-data\nlisten.mode = 0660\npm = ondemand\npm.max_children = 4\npm.process_idle_timeout = 10s\npm.max_requests = 500\nclear_env = yes\nsecurity.limit_extensions = .php\ncatch_workers_output = yes\n", key, key, key, quote(socket))
	allowed := []string{p.Path, filepath.Join(root, "tmp")}
	seen := map[string]bool{}
	for _, d := range p.DataDirs {
		if err := ValidateDirectory(d); err != nil {
			return "", err
		}
		if seen[d.Env] || seen["name:"+d.Name] {
			return "", errors.New("имя каталога или переменная уже используется")
		}
		seen[d.Env], seen["name:"+d.Name] = true, true
		path := filepath.Join(root, "volumes", d.Name)
		allowed = append(allowed, path)
		fmt.Fprintf(&b, "env[%s] = %s\n", d.Env, quote(path))
	}
	fmt.Fprintf(&b, "php_admin_value[open_basedir] = %s\nphp_admin_value[upload_tmp_dir] = %s\nphp_admin_value[sys_temp_dir] = %s\nphp_admin_value[session.save_path] = %s\nphp_admin_flag[log_errors] = on\nphp_admin_flag[display_errors] = off\n", quote(strings.Join(allowed, ":")), quote(filepath.Join(root, "tmp")), quote(filepath.Join(root, "tmp")), quote(filepath.Join(root, "tmp")))
	return b.String(), nil
}

// Ensure provisions an idempotent pool and rolls its config back on test/reload failure.
func (m *Manager) Ensure(p *store.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.check(); err != nil {
		return err
	}
	root := store.ProjectDataRoot(m.DataDir, p)
	config, socket := m.paths(p)
	text, err := Render(p, root, socket)
	if err != nil {
		return err
	}
	key := store.ProjectKey(p)
	u, err := user.Lookup(key)
	if err != nil {
		var unknown user.UnknownUserError
		if !errors.As(err, &unknown) {
			return err
		}
		if err := command("useradd", "--system", "--user-group", "--no-create-home", "--home-dir", root, "--shell", "/usr/sbin/nologin", key); err != nil {
			return err
		}
		u, err = user.Lookup(key)
		if err != nil {
			return err
		}
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return err
	}
	// Parent directories stay root-owned: workers cannot replace volume roots.
	for _, dir := range []string{root, filepath.Join(root, "volumes")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	dirs := []string{filepath.Join(root, "tmp")}
	for _, d := range p.DataDirs {
		dirs = append(dirs, filepath.Join(root, "volumes", d.Name))
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		fi, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return errors.New("каталог данных не должен быть ссылкой")
		}
		if err := os.Chown(dir, uid, gid); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return err
		}
	}
	if !p.Enabled {
		return nil
	}
	previous, err := os.ReadFile(config)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if string(previous) == text {
		return nil
	}
	if err := os.WriteFile(config+".tmp", []byte(text), 0600); err != nil {
		return err
	}
	if err := os.Rename(config+".tmp", config); err != nil {
		return err
	}
	rollback := func() {
		if previous == nil {
			_ = os.Remove(config)
		} else {
			_ = os.WriteFile(config, previous, 0600)
		}
	}
	if err := command("php-fpm"+m.Version, "-t"); err != nil {
		rollback()
		return err
	}
	if err := command("systemctl", "reload", "php"+m.Version+"-fpm"); err != nil {
		rollback()
		_ = command("systemctl", "reload", "php"+m.Version+"-fpm")
		return err
	}
	return nil
}

// Disable removes a pool but preserves all persistent project data.
func (m *Manager) Disable(p *store.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || !versionRE.MatchString(m.Version) {
		return errors.New("управление PHP-FPM требует root на Linux и корректную --php-version")
	}
	config, _ := m.paths(p)
	previous, err := os.ReadFile(config)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.Remove(config); err != nil {
		return err
	}
	if err := command("php-fpm"+m.Version, "-t"); err != nil {
		_ = os.WriteFile(config, previous, 0600)
		return err
	}
	if err := command("systemctl", "reload", "php"+m.Version+"-fpm"); err != nil {
		_ = os.WriteFile(config, previous, 0600)
		return err
	}
	return nil
}
