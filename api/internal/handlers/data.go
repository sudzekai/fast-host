package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"hosting/internal/httpx"
	"hosting/internal/phpfpm"
	"hosting/internal/store"
	"hosting/internal/usage"
)

var dataFileRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

func (h *Handlers) setProvider(w http.ResponseWriter, r *http.Request) {
	h.projectMu.Lock()
	defer h.projectMu.Unlock()
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	var req struct {
		Provider string `json:"provider"`
	}
	if httpx.ReadJSON(w, r, &req) != nil || (req.Provider != store.ProviderPHP && req.Provider != store.ProviderNginx) {
		httpx.Error(w, 400, "выберите nginx или php-fpm")
		return
	}
	if req.Provider == store.ProviderNginx && len(p.DataDirs) > 0 {
		httpx.Error(w, 400, "у проекта подключены каталоги данных PHP")
		return
	}
	previous := *p
	p.Provider = req.Provider
	if p.Provider == store.ProviderPHP && p.Enabled {
		if err := h.php.Ensure(p); err != nil {
			httpx.Error(w, 502, h.hidePaths(r, err.Error()))
			return
		}
	}
	if err := h.nginx.Apply(h.site(p), p.Enabled); err != nil {
		if p.Provider == store.ProviderPHP && previous.Provider != store.ProviderPHP {
			_ = h.php.Disable(p)
		}
		httpx.Error(w, 502, h.hidePaths(r, err.Error()))
		return
	}
	if previous.Provider == store.ProviderPHP && p.Provider != store.ProviderPHP {
		if err := h.php.Disable(p); err != nil {
			_ = h.nginx.Apply(h.site(&previous), previous.Enabled)
			httpx.Error(w, 502, h.hidePaths(r, err.Error()))
			return
		}
	}
	if err := h.store.UpdateProject(p); err != nil {
		if previous.Provider == store.ProviderPHP && previous.Enabled {
			_ = h.php.Ensure(&previous)
		} else if p.Provider == store.ProviderPHP {
			_ = h.php.Disable(p)
		}
		_ = h.nginx.Apply(h.site(&previous), previous.Enabled)
		httpx.Error(w, 500, "ошибка сохранения провайдера")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"project": h.publicProject(r, p)})
}

func (h *Handlers) listDataDirs(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	result := make([]map[string]any, 0, len(p.DataDirs))
	for _, d := range p.DataDirs {
		if phpfpm.ValidateDirectory(d) != nil {
			httpx.Error(w, 500, "некорректный каталог данных")
			return
		}
		path := filepath.Join(store.ProjectDataRoot(h.cfg.DataDir, p), "volumes", d.Name)
		root, err := os.OpenRoot(path)
		if err != nil {
			httpx.Error(w, 500, "каталог данных недоступен")
			return
		}
		f, err := root.Open(".")
		if err != nil {
			root.Close()
			httpx.Error(w, 500, "каталог данных недоступен")
			return
		}
		entries, err := f.ReadDir(-1)
		f.Close()
		root.Close()
		if err != nil {
			httpx.Error(w, 500, "не удалось прочитать каталог данных")
			return
		}
		files := []string{}
		for _, entry := range entries {
			if entry.Type().IsRegular() {
				files = append(files, entry.Name())
			}
		}
		result = append(result, map[string]any{"name": d.Name, "env": d.Env, "path": path, "files": files})
	}
	httpx.WriteJSON(w, 200, map[string]any{"directories": result})
}

func (h *Handlers) createDataDir(w http.ResponseWriter, r *http.Request) {
	h.projectMu.Lock()
	defer h.projectMu.Unlock()
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	if p.Provider != store.ProviderPHP {
		httpx.Error(w, 400, "каталоги данных доступны для PHP-FPM проектов")
		return
	}
	var req store.DataDirectory
	if httpx.ReadJSON(w, r, &req) != nil {
		httpx.Error(w, 400, "некорректный запрос")
		return
	}
	if err := phpfpm.ValidateDirectory(req); err != nil {
		httpx.Error(w, 400, err.Error())
		return
	}
	if len(p.DataDirs) >= 16 {
		httpx.Error(w, 400, "не более 16 каталогов на проект")
		return
	}
	for _, d := range p.DataDirs {
		if d.Name == req.Name || d.Env == req.Env {
			httpx.Error(w, 409, "имя каталога или переменная уже используется")
			return
		}
	}
	previous := *p
	p.DataDirs = append(append([]store.DataDirectory(nil), p.DataDirs...), req)
	if err := h.php.Ensure(p); err != nil {
		httpx.Error(w, 502, h.hidePaths(r, err.Error()))
		return
	}
	if !p.Enabled {
		if err := h.php.Disable(p); err != nil {
			httpx.Error(w, 502, h.hidePaths(r, err.Error()))
			return
		}
	}
	if err := h.store.UpdateProject(p); err != nil {
		if previous.Enabled {
			_ = h.php.Ensure(&previous)
		}
		httpx.Error(w, 500, "ошибка сохранения каталога")
		return
	}
	httpx.WriteJSON(w, 201, map[string]any{"project": h.publicProject(r, p)})
}

func (h *Handlers) openDataRoot(p *store.Project, name string) (*os.Root, error) {
	for _, d := range p.DataDirs {
		if d.Name == name && phpfpm.ValidateDirectory(d) == nil {
			return os.OpenRoot(filepath.Join(store.ProjectDataRoot(h.cfg.DataDir, p), "volumes", d.Name))
		}
	}
	return nil, store.ErrNotFound
}

func (h *Handlers) dataFile(w http.ResponseWriter, r *http.Request) {
	h.projectMu.Lock()
	defer h.projectMu.Unlock()
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	name := r.PathValue("file")
	if !dataFileRE.MatchString(name) {
		httpx.Error(w, 400, "некорректное имя файла")
		return
	}
	root, err := h.openDataRoot(p, r.PathValue("directory"))
	if err != nil {
		httpx.Error(w, 404, "каталог не найден")
		return
	}
	defer root.Close()
	if r.Method == http.MethodGet {
		f, err := root.Open(name)
		if err != nil {
			httpx.Error(w, 404, "файл не найден")
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			httpx.Error(w, 400, "редактор поддерживает обычные файлы до 1 МБ")
			return
		}
		content, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
		if err != nil || len(content) > 1<<20 {
			httpx.Error(w, 500, "ошибка чтения файла")
			return
		}
		httpx.WriteJSON(w, 200, map[string]any{"content": string(content)})
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || len(req.Content) > 1<<20 {
		httpx.Error(w, 400, "ожидается текстовый файл до 1 МБ")
		return
	}
	// Create a new inode and rename it: never overwrite a SQLite DB or follow a
	// worker-created symlink/hardlink while writing as root.
	if info, err := root.Lstat(name); err == nil && (!info.Mode().IsRegular() || filepath.Ext(name) == ".sqlite" || filepath.Ext(name) == ".db") {
		httpx.Error(w, 400, "файл нельзя редактировать")
		return
	}
	owner, err := h.store.GetUserDetailed(p.UserID)
	if err != nil {
		httpx.Error(w, 500, "ошибка квоты")
		return
	}
	oldSize := int64(0)
	if info, err := root.Stat(name); err == nil {
		oldSize = info.Size()
	}
	if int64(len(req.Content))-oldSize > owner.QuotaBytes-owner.UsedBytes {
		httpx.Error(w, 413, "квота хранения исчерпана")
		return
	}
	u, err := user.Lookup(store.ProjectKey(p))
	if err != nil {
		httpx.Error(w, 500, "пользователь пула не найден")
		return
	}
	uid, e1 := strconv.Atoi(u.Uid)
	gid, e2 := strconv.Atoi(u.Gid)
	if e1 != nil || e2 != nil {
		httpx.Error(w, 500, "некорректный пользователь пула")
		return
	}
	tmp := fmt.Sprintf(".panel-%d.tmp", time.Now().UnixNano())
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		httpx.Error(w, 500, "не удалось создать файл")
		return
	}
	defer root.Remove(tmp)
	_, writeErr := f.WriteString(req.Content)
	if writeErr == nil {
		writeErr = f.Chown(uid, gid)
	}
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = root.Rename(tmp, name)
	}
	if writeErr != nil {
		httpx.Error(w, 500, "ошибка сохранения файла")
		return
	}
	measured, err := usage.Measure(h.cfg.DataDir, p)
	if err != nil {
		httpx.Error(w, 500, "файл сохранён, не удалось измерить размер данных")
		return
	}
	p.DiskBytes = measured.TotalBytes
	if err := h.store.UpdateProjectDisk(p.ID, p.DiskBytes); err != nil && !errors.Is(err, store.ErrNotFound) {
		httpx.Error(w, 500, "файл сохранён, ошибка обновления размера")
		return
	}
	httpx.WriteJSON(w, 200, map[string]bool{"ok": true})
}
