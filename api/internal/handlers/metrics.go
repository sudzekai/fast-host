package handlers

import (
	"hosting/internal/httpx"
	"hosting/internal/store"
	"net/http"
	"time"
)

func metricTime(value string) time.Time {
	t, _ := time.ParseInLocation(store.TimeFormat, value, time.Local)
	return t
}
func (h *Handlers) projectMetrics(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	series, err := h.store.ListMetrics(p.ID, 120)
	if err != nil {
		httpx.Error(w, 500, "ошибка чтения метрик")
		return
	}
	points := make([]map[string]any, 0, len(series))
	for _, m := range series {
		points = append(points, map[string]any{"ts": m.TS, "timestamp_ms": metricTime(m.TS).UnixMilli(), "disk_bytes": m.DiskBytes, "requests_cum": m.RequestsCum, "bandwidth_cum": m.BandwidthCum})
	}
	last, err := h.store.LastMetric(p.ID)
	if err != nil && err != store.ErrNotFound {
		httpx.Error(w, 500, "ошибка чтения метрик")
		return
	}
	interval := time.Duration(h.cfg.MetricsIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	status := "pending"
	var updated any
	var age any
	var requests, bandwidth int64
	if last != nil {
		sampled := metricTime(last.TS)
		updated = sampled.Format(time.RFC3339)
		age = int64(time.Since(sampled).Seconds())
		status = "ok"
		if time.Since(sampled) > 3*interval {
			status = "stale"
		}
		requests, bandwidth = last.RequestsCum, last.BandwidthCum
	}
	response := map[string]any{"project_id": p.ID, "disk_bytes": p.DiskBytes, "requests_cum": requests, "bandwidth_cum": bandwidth, "series": points, "updated_at": updated, "age_seconds": age, "status": status, "interval_seconds": int64(interval.Seconds()), "can_refresh": h.sampler != nil, "server_time": time.Now().Format(time.RFC3339), "collector": "external"}
	if h.sampler != nil {
		current := h.sampler.Status(p.ID)
		response["collector"] = "embedded"
		response["running"] = current.Running
		response["duration_ms"] = current.DurationMS
		if current.Error != "" {
			response["status"] = "error"
			response["error"] = h.hidePaths(r, current.Error)
		}
		if !current.LastSuccess.IsZero() {
			response["storage"] = current.Usage
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, 200, response)
}

func (h *Handlers) refreshProjectMetrics(w http.ResponseWriter, r *http.Request) {
	h.projectMu.Lock()
	defer h.projectMu.Unlock()
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	if h.sampler == nil {
		httpx.Error(w, 409, "встроенный сборщик выключен; метрики обновляет отдельный сервис")
		return
	}
	if err := h.sampler.SampleProject(p); err != nil {
		httpx.Error(w, 500, "не удалось обновить метрики: "+h.hidePaths(r, err.Error()))
		return
	}
	h.projectMetrics(w, r)
}

func (h *Handlers) usageStatus(projects []store.Project) map[string]any {
	var oldest time.Time
	pending, stale, failed := 0, 0, 0
	interval := time.Duration(h.cfg.MetricsIntervalSec) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	for _, p := range projects {
		last, err := h.store.LastMetric(p.ID)
		if err != nil && err != store.ErrNotFound {
			failed++
			continue
		}
		if err != nil || last == nil {
			pending++
			continue
		}
		sampled := metricTime(last.TS)
		if oldest.IsZero() || sampled.Before(oldest) {
			oldest = sampled
		}
		if time.Since(sampled) > 3*interval {
			stale++
		}
		if h.sampler != nil && h.sampler.Status(p.ID).Error != "" {
			failed++
		}
	}
	var updated any
	if !oldest.IsZero() {
		updated = oldest.Format(time.RFC3339)
	}
	return map[string]any{"updated_at": updated, "pending_projects": pending, "stale_projects": stale, "error_projects": failed, "interval_seconds": int64(interval.Seconds()), "can_refresh": h.sampler != nil}
}

func (h *Handlers) refreshUsage(w http.ResponseWriter, r *http.Request) {
	h.projectMu.Lock()
	defer h.projectMu.Unlock()
	if h.sampler == nil {
		httpx.Error(w, 409, "встроенный сборщик метрик выключен")
		return
	}
	projects, err := h.store.ListProjectsByUser(httpx.UserFrom(r).ID)
	if err != nil {
		httpx.Error(w, 500, "ошибка базы данных")
		return
	}
	for i := range projects {
		if err := h.sampler.SampleProject(&projects[i]); err != nil {
			httpx.Error(w, 500, "не удалось обновить использование диска: "+h.hidePaths(r, err.Error()))
			return
		}
	}
	h.me(w, r)
}
