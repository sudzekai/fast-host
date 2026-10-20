package metrics

import (
	"bufio"
	"context"
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"hosting/internal/store"
)

type Sampler struct {
	store    store.Storage
	dataDir  string
	interval time.Duration
	offsets  map[int64]int64
	mu       sync.Mutex
}

func New(st store.Storage, dataDir string, interval time.Duration) *Sampler {
	return &Sampler{store: st, dataDir: dataDir, interval: interval, offsets: map[int64]int64{}}
}

func (s *Sampler) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.SampleAll()
		}
	}
}

func (s *Sampler) SampleAll() {
	projects, err := s.store.ListAllProjects()
	if err != nil {
		log.Printf("metrics: list projects: %v", err)
		return
	}
	for i := range projects {
		p := projects[i]
		if err := s.SampleProject(&p); err != nil && !errors.Is(err, store.ErrNotFound) {
			log.Printf("metrics: project %s: %v", p.Name, err)
		}
	}
}

// Forget сбрасывает состояние удалённого проекта.
func (s *Sampler) Forget(projectID int64) {
	s.mu.Lock()
	delete(s.offsets, projectID)
	s.mu.Unlock()
	_ = s.store.DeleteSetting(offsetKey(projectID))
}

func offsetKey(projectID int64) string { return "log_offset:" + strconv.FormatInt(projectID, 10) }

// offset хранится в БД, иначе после перезапуска access.log читался бы с начала
// и запросы прибавлялись к накопленным счётчикам повторно.
func (s *Sampler) offset(projectID int64) int64 {
	s.mu.Lock()
	off, ok := s.offsets[projectID]
	s.mu.Unlock()
	if ok {
		return off
	}
	if v, err := s.store.GetSetting(offsetKey(projectID)); err == nil {
		off, _ = strconv.ParseInt(v, 10, 64)
	}
	return off
}

func (s *Sampler) setOffset(projectID, off int64) {
	s.mu.Lock()
	prev, ok := s.offsets[projectID]
	s.offsets[projectID] = off
	s.mu.Unlock()
	if !ok || prev != off {
		_ = s.store.SetSetting(offsetKey(projectID), strconv.FormatInt(off, 10))
	}
}

func (s *Sampler) SampleProject(p *store.Project) error {
	disk := dirSize(p.Path)
	dreq, dbw, err := s.accessLogDelta(p.ID, filepath.Join(p.Path, ".conf", "access.log"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	last, err := s.store.LastMetric(p.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	var creq, cbw int64
	if last != nil {
		creq, cbw = last.RequestsCum, last.BandwidthCum
	}
	m := &store.Metric{
		ProjectID:    p.ID,
		TS:           store.Now(),
		DiskBytes:    disk,
		RequestsCum:  creq + dreq,
		BandwidthCum: cbw + dbw,
	}
	if err := s.store.AddMetric(m); err != nil {
		return err
	}
	return s.store.UpdateProjectDisk(p.ID, disk)
}

var reqRe = regexp.MustCompile(`"\s+\d{3}\s+(\d+)\s+"`)

func (s *Sampler) accessLogDelta(projectID int64, path string) (requests, bytes int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}
	off := s.offset(projectID)
	if off > stat.Size() {
		off = 0 // лог ротирован или усечён
	}
	if off >= stat.Size() {
		s.setOffset(projectID, off)
		return 0, 0, nil
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return 0, 0, err
	}
	// Читаем только целые строки: последняя может быть ещё недописана nginx.
	r := bufio.NewReaderSize(f, 64*1024)
	read := off
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break
		}
		read += int64(len(line))
		if len(line) <= 1 {
			continue
		}
		requests++
		if m := reqRe.FindSubmatch(line); len(m) == 2 {
			if n, err := strconv.ParseInt(string(m[1]), 10, 64); err == nil {
				bytes += n
			}
		}
	}
	s.setOffset(projectID, read)
	return requests, bytes, nil
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
