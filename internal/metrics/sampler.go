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
		if err := s.SampleProject(&p); err != nil {
			log.Printf("metrics: project %s: %v", p.Name, err)
		}
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
		TS:           time.Now().Format("2006-01-02 15:04:05"),
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
	s.mu.Lock()
	off := s.offsets[projectID]
	if off > stat.Size() {
		off = 0
	}
	s.mu.Unlock()
	if off >= stat.Size() {
		return 0, 0, nil
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return 0, 0, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		requests++
		if m := reqRe.FindSubmatch(line); len(m) == 2 {
			if n, err := strconv.ParseInt(string(m[1]), 10, 64); err == nil {
				bytes += n
			}
		}
	}
	if err := sc.Err(); err != nil {
		return 0, 0, err
	}
	s.mu.Lock()
	s.offsets[projectID] = stat.Size()
	s.mu.Unlock()
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