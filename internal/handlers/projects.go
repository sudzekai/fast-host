package handlers

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
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
		"projects":   projects,
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
	if _, err := h.store.GetProjectByDomain(req.Domain); err == nil {
		httpx.Error(w, http.StatusConflict, "домен уже используется другим проектом")
		return
	}
	dir := projectDir(h.cfg.DataDir, u.Username, req.Name)
	if _, err := os.Stat(dir); err == nil {
		httpx.Error(w, http.StatusConflict, "проект с таким именем уже существует")
		return
	}
	if err := nginx.WriteFiles(dir, req.Domain, h.cfg.CertbotEmail, u.Username+"/"+req.Name); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка создания конфигурации: "+err.Error())
		return
	}
	p := &store.Project{
		UserID: u.ID, Name: req.Name, Domain: req.Domain,
		Status: store.StatusActive, Path: dir,
	}
	if err := h.store.CreateProject(p); err != nil {
		httpx.Error(w, httpx.ErrStatus(err), "не удалось создать проект")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"project": p})
}

func (h *Handlers) getProject(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	if p.Username == "" {
		if owner, err := h.store.GetUserByID(p.UserID); err == nil {
			p.Username = owner.Username
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"project": p})
}

func (h *Handlers) deleteProject(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	if err := os.RemoveAll(p.Path); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка удаления файлов")
		return
	}
	if err := h.store.DeleteProject(p.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handlers) uploadProjectZip(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	u := httpx.UserFrom(r)
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

	projects, err := h.store.ListProjectsByUser(u.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	var used int64
	for _, pr := range projects {
		used += pr.DiskBytes
	}
	quotaLeft := u.QuotaBytes - used
	if quotaLeft <= 0 {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "квота хранения исчерпана")
		return
	}

	tmp, err := os.CreateTemp(filepath.Join(p.Path, ".conf"), "upload-*.zip")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка временного файла")
		return
	}
	defer os.Remove(tmp.Name())
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
	p.DiskBytes = dirSize(p.Path)
	if err := h.store.UpdateProjectDisk(p.ID, p.DiskBytes); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"project":    p,
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

func (h *Handlers) getNginxConfig(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	cfg, err := nginx.ReadConfig(p.Path)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "конфиг не найден")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"config": cfg})
}

type sslResp struct {
	Message    string `json:"message"`
	SSLEnabled bool   `json:"ssl_enabled"`
	Live       bool   `json:"live"`
}

func (h *Handlers) applySSL(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	out, err := nginx.ApplyCertbot(p.Path, p.Domain, h.cfg.CertbotEmail, h.cfg.EnableLiveSSL)
	if err != nil {
		p.Status = store.StatusError
		_ = h.store.UpdateProject(p)
		httpx.Error(w, http.StatusInternalServerError, out)
		return
	}
	p.Status = store.StatusActive
	if h.cfg.EnableLiveSSL {
		p.SSLEnabled = true
	}
	_ = h.store.UpdateProject(p)
	httpx.WriteJSON(w, http.StatusOK, sslResp{Message: out, SSLEnabled: p.SSLEnabled, Live: h.cfg.EnableLiveSSL})
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