package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"hosting/internal/httpx"
	"hosting/internal/nginx"
	"hosting/internal/store"
)

// Файлы, которые генерировали прежние версии панели: теперь всё делает сама панель.
var obsoleteConfFiles = []string{"nginx.conf", "install.sh", "certbot.sh"}

func (h *Handlers) site(p *store.Project) nginx.Site {
	s := nginx.Site{Domain: p.Domain, Root: p.Path, LogDir: filepath.Join(p.Path, ".conf")}
	if p.SSLEnabled {
		if c, err := h.nginx.CurrentCert(p.Domain); err == nil {
			s.SSL, s.CertFile, s.KeyFile = true, c.CertFile, c.KeyFile
		}
	}
	return s
}

func (h *Handlers) siteURL(p *store.Project) string {
	opt := h.nginx.Options()
	if p.SSLEnabled {
		if opt.HTTPSPort != 443 {
			return "https://" + p.Domain + ":" + strconv.Itoa(opt.HTTPSPort)
		}
		return "https://" + p.Domain
	}
	if opt.HTTPPort != 80 {
		return "http://" + p.Domain + ":" + strconv.Itoa(opt.HTTPPort)
	}
	return "http://" + p.Domain
}

func setCert(p *store.Project, c *nginx.Cert) {
	p.SSLEnabled = true
	p.SSLIssuedAt = c.NotBefore.Local().Format(store.TimeFormat)
	p.SSLExpiresAt = c.NotAfter.Local().Format(store.TimeFormat)
}

// SyncSites при старте перегенерирует конфиги всех проектов (шаблон мог измениться)
// и один раз перезагружает nginx.
func (h *Handlers) SyncSites() {
	projects, err := h.store.ListAllProjects()
	if err != nil {
		log.Printf("nginx: список проектов: %v", err)
		return
	}
	for i := range projects {
		p := &projects[i]
		for _, f := range obsoleteConfFiles {
			_ = os.Remove(filepath.Join(p.Path, ".conf", f))
		}
		if p.SSLEnabled {
			if c, err := h.nginx.CurrentCert(p.Domain); err == nil {
				if p.SSLExpiresAt == "" {
					setCert(p, c)
					_ = h.store.UpdateProject(p)
				}
			} else {
				log.Printf("nginx: %s: сертификат не найден, сайт работает по HTTP", p.Domain)
			}
		}
		if err := h.nginx.Write(h.site(p), p.Enabled); err != nil {
			log.Printf("nginx: %s: %v", p.Domain, err)
		}
	}
	if err := h.nginx.ReloadAll(); err != nil {
		log.Printf("nginx: перезагрузка не удалась, сайты не применены: %v", err)
		return
	}
	log.Printf("nginx: применено проектов: %d, каталог конфигов: %s", len(projects), h.nginx.Options().SitesDir)
}

// RenewCertificates периодически перевыпускает сертификаты, до истечения которых меньше 30 дней.
func (h *Handlers) RenewCertificates(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		projects, err := h.store.ListAllProjects()
		if err != nil {
			continue
		}
		for i := range projects {
			p := &projects[i]
			if !p.SSLEnabled || !p.Enabled {
				continue
			}
			exp, err := time.ParseInLocation(store.TimeFormat, p.SSLExpiresAt, time.Local)
			if err == nil && time.Until(exp) > 30*24*time.Hour {
				continue
			}
			if err := h.issueSSL(p); err != nil {
				log.Printf("ssl: продление %s: %v", p.Domain, err)
			} else {
				log.Printf("ssl: %s продлён до %s", p.Domain, p.SSLExpiresAt)
			}
		}
	}
}

// issueSSL выпускает сертификат и переводит сайт на HTTPS с редиректом 80 → 443.
func (h *Handlers) issueSSL(p *store.Project) error {
	if !p.Enabled {
		return errors.New("проект выключен: включите его перед выпуском сертификата")
	}
	// Let's Encrypt проверяет домен по HTTP, поэтому сайт должен уже обслуживаться nginx.
	if err := h.nginx.Apply(h.site(p), true); err != nil {
		return err
	}
	c, err := h.nginx.Issue(p.Domain)
	if err != nil {
		return err
	}
	prev := *p
	setCert(p, c)
	if err := h.nginx.Apply(h.site(p), true); err != nil {
		*p = prev
		return err
	}
	p.Status = store.StatusActive
	return h.store.UpdateProject(p)
}

type sslResp struct {
	Project  map[string]any `json:"project"`
	Provider string         `json:"provider"`
	CAFile   string         `json:"ca_file,omitempty"`
}

func (h *Handlers) applySSL(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	if err := h.issueSSL(p); err != nil {
		log.Printf("ssl: %s: %v", p.Domain, err)
		httpx.Error(w, http.StatusBadGateway, "не удалось выпустить сертификат: "+h.hidePaths(r, err.Error()))
		return
	}
	resp := sslResp{Project: h.publicProject(r, p), Provider: h.nginx.Provider(p.Domain)}
	if resp.Provider == nginx.ProviderSelfSigned && isAdmin(r) {
		resp.CAFile = h.nginx.CAFile()
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

type enabledReq struct {
	Enabled bool `json:"enabled"`
}

func (h *Handlers) setProjectEnabled(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	var req enabledReq
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	if err := h.nginx.Apply(h.site(p), req.Enabled); err != nil {
		log.Printf("nginx: %s: %v", p.Domain, err)
		httpx.Error(w, http.StatusBadGateway, "nginx не применил конфигурацию: "+h.hidePaths(r, err.Error()))
		return
	}
	p.Enabled = req.Enabled
	p.Status = store.StatusActive
	if err := h.store.UpdateProject(p); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"project": h.publicProject(r, p)})
}

func (h *Handlers) getNginxConfig(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"config": h.nginx.Render(h.site(p)), "enabled": p.Enabled})
}
