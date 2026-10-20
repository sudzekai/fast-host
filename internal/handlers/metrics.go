package handlers

import (
	"net/http"

	"hosting/internal/httpx"
)

func (h *Handlers) projectMetrics(w http.ResponseWriter, r *http.Request) {
	p := h.loadProject(w, r)
	if p == nil {
		return
	}
	series, err := h.store.ListMetrics(p.ID, 120)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "ошибка базы данных")
		return
	}
	last, err := h.store.LastMetric(p.ID)
	if err != nil {
		last = nil
	}
	reqCum, bwCum := int64(0), int64(0)
	if last != nil {
		reqCum, bwCum = last.RequestsCum, last.BandwidthCum
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"project_id":    p.ID,
		"disk_bytes":    p.DiskBytes,
		"requests_cum":  reqCum,
		"bandwidth_cum": bwCum,
		"series":        series,
	})
}