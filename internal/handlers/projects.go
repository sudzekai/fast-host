package handlers

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"hosting/internal/httpx"
	"hosting/internal/nginx"
	"hosting/internal/store"
)

type createProjectReq struct {
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

func (h *Handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	u := httpx.UserFrom(r)
	projects, err := h.store.ListProjectsByUser(u.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	var used int64
	for i := range projects {
		projects[i].Username = u.Username
		used += projects[i].DiskBytes
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"projects":   h.publicProjects(r, projects),
		"used_bytes": used,
		"quota":      u.QuotaBytes,
	})
}

func (h *Handlers) createProject(w http.ResponseWriter, r *http.Request) {
	u := httpx.UserFrom(r)
	var req createProjectReq
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	req.Name = lowerASCII(req.Name)
	req.Domain = lowerASCII(req.Domain)
	if !validProjectName(req.Name) {
		httpx.Error(w, http.StatusBadRequest, "имя проекта: латиница в нижнем регистре, 2-64 символа (a-z, 0-9, _-)")
		return
	}
	if err := nginx.ValidateDomain(req.Domain); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	// www.example.com и example.com обслуживаются одним server-блоком — это один сайт.
	for _, d := range nginx.Aliases(req.Domain) {
		if _, err := h.store.GetProjectByDomain(d); err == nil {
			httpx.Error(w, http.StatusConflict, "домен уже используется другим проектом")
			return
		}
	}
	dir := projectDir(h.cfg.DataDir, u.Username, req.Name)
	if _, err := os.Stat(dir); err == nil {
		httpx.Error(w, http.StatusConflict, "проект с таким именем уже существует")
		return
	}
	if err := os.MkdirAll(filepath.Join(dir, ".conf"), 0o755); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "не удалось создать каталог проекта")
		return
	}
	p := &store.Project{
		UserID: u.ID, Username: u.Username, Name: req.Name, Domain: req.Domain,
		Status: store.StatusActive, Enabled: true, Path: dir,
	}
	if err := h.nginx.Apply(h.site(p), true); err != nil {
		os.RemoveAll(dir)
		log.Printf("nginx: %s: %v", p.Domain, err)
		httpx.Error(w, http.StatusBadGateway, "nginx не применил конфигурацию: "+h.hidePaths(r, err.Error()))
		return
	}
	if err := h.store.CreateProject(p); err != nil {
		_ = h.nginx.Apply(h.site(p), false)
		os.RemoveAll(dir)
		httpx.Error(w, httpx.ErrStatus(err), "не удалось создать проект")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"project": h.publicProject(r, p)})
}

func (h *Handlers) getProject(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"project": h.publicProject(r, p)})
}

func (h *Handlers) deleteProject(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	if !h.insideUsersDir(p.Path) {
		log.Printf("delete: путь проекта %d вне каталога users: %q", p.ID, p.Path)
		httpx.Error(w, http.StatusInternalServerError, "некорректный путь проекта, обратитесь к администратору")
		return
	}
	// Сначала убираем сайт из nginx: он не должен указывать на удаляемый каталог.
	if err := h.nginx.Remove(p.Domain); err != nil {
		log.Printf("nginx: удаление %s: %v", p.Domain, err)
	}
	if err := os.RemoveAll(p.Path); err != nil {
		log.Printf("delete: %s: %v", p.Path, err)
		httpx.Error(w, http.StatusInternalServerError, "ошибка удаления файлов")
		return
	}
	if err := h.store.DeleteProject(p.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	if h.sampler != nil {
		h.sampler.Forget(p.ID)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handlers) uploadProjectZip(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	// Квота — владельца проекта, даже если загружает администратор.
	owner, err := h.store.GetUserByID(p.UserID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "владелец проекта не найден")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<30)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "ошибка чтения формы: "+err.Error())
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "поле file (zip-архив) обязательно")
		return
	}
	defer file.Close()

	projects, err := h.store.ListProjectsByUser(owner.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	var used int64
	for _, pr := range projects {
		used += pr.DiskBytes
	}
	quotaLeft := owner.QuotaBytes - used
	if quotaLeft <= 0 {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "квота хранения исчерпана")
		return
	}

	tmp, err := os.CreateTemp(filepath.Join(p.Path, ".conf"), "upload-*.zip")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка временного файла")
		return
	}
	defer os.Remove(tmp.Name()) // на случай раннего выхода; ниже удаляется явно до подсчёта размера
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		httpx.Error(w, http.StatusBadRequest, "ошибка чтения архива")
		return
	}
	tmp.Close()
	stat, err := os.Stat(tmp.Name())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка чтения архива")
		return
	}
	if stat.Size() == 0 {
		httpx.Error(w, http.StatusBadRequest, "архив пуст")
		return
	}

	staging := filepath.Join(p.Path, ".conf", ".upload-staging")
	os.RemoveAll(staging)
	zf, err := os.Open(tmp.Name())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка чтения архива")
		return
	}
	written, err := extractZip(zf, stat.Size(), staging, quotaLeft)
	zf.Close()
	if err != nil {
		os.RemoveAll(staging)
		if errors.Is(err, store.ErrQuotaExceeded) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "архив превышает оставшуюся квоту ("+formatBytes(quotaLeft)+")")
			return
		}
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if !hasIndex(staging) {
		os.RemoveAll(staging)
		httpx.Error(w, http.StatusBadRequest, errNoIndex.Error())
		return
	}
	if err := mergeDir(staging, p.Path); err != nil {
		os.RemoveAll(staging)
		httpx.Error(w, http.StatusInternalServerError, "ошибка переноса файлов")
		return
	}
	os.RemoveAll(staging)
	os.Remove(tmp.Name())
	p.DiskBytes = dirSize(p.Path)
	p.UpdatedAt = store.Now()
	if err := h.store.UpdateProject(p); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"project":    h.publicProject(r, p),
		"written":    written,
		"disk_bytes": p.DiskBytes,
	})
}

func (h *Handlers) downloadProjectZip(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+p.Name+".zip")
	zw := zip.NewWriter(w)
	defer zw.Close()
	_ = filepath.Walk(p.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(p.Path, path)
		if err != nil {
			return nil
		}
		if rel == ".conf" || strings.HasPrefix(rel, ".conf"+string(os.PathSeparator)) {
			return nil
		}
		fw, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(fw, src)
		return err
	})
}

type fileEntry struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
	Size int64  `json:"size"`
}

func (h *Handlers) listFiles(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	var out []fileEntry
	_ = filepath.Walk(p.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(p.Path, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		if rel == ".conf" || strings.HasPrefix(rel, ".conf"+string(os.PathSeparator)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		out = append(out, fileEntry{Path: filepath.ToSlash(rel), Dir: info.IsDir(), Size: info.Size()})
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return out[i].Path < out[j].Path
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"files": out})
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmtBytesInt(n) + " Б"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %s", float64(n)/float64(div), []string{"КБ", "МБ", "ГБ", "ТБ"}[exp])
}

func fmtBytesInt(n int64) string {
	if n == 0 {
		return "0"
	}
	return itoa(n)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
