package metrics

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"hosting/internal/store"
	"hosting/internal/usage"
)

type Status struct {
	Running     bool            `json:"running"`
	Error       string          `json:"error,omitempty"`
	LastAttempt time.Time       `json:"last_attempt"`
	LastSuccess time.Time       `json:"last_success"`
	DurationMS  int64           `json:"duration_ms"`
	Usage       usage.Breakdown `json:"usage"`
}

type Sampler struct {
	store    store.Storage
	dataDir  string
	interval time.Duration
	sampleMu sync.Mutex
	mu       sync.Mutex
	statuses map[int64]Status
}

func New(st store.Storage, dataDir string, interval time.Duration) *Sampler {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Sampler{store: st, dataDir: dataDir, interval: interval, statuses: map[int64]Status{}}
}
func (s *Sampler) Interval() time.Duration { return s.interval }
func (s *Sampler) Status(id int64) Status  { s.mu.Lock(); defer s.mu.Unlock(); return s.statuses[id] }
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
		if err := s.SampleProject(&projects[i]); err != nil && !errors.Is(err, store.ErrNotFound) {
			log.Printf("metrics: project %s: %v", projects[i].Name, err)
		}
	}
}
func (s *Sampler) Forget(id int64) {
	s.sampleMu.Lock()
	defer s.sampleMu.Unlock()
	s.mu.Lock()
	delete(s.statuses, id)
	s.mu.Unlock()
	_ = s.store.DeleteSetting(offsetKey(id))
}
func offsetKey(id int64) string { return "log_offset:" + strconv.FormatInt(id, 10) }

func (s *Sampler) SampleProject(project *store.Project) (err error) {
	s.sampleMu.Lock()
	defer s.sampleMu.Unlock()
	// Re-read to avoid sampling stale provider settings or a deleted project.
	p, err := s.store.GetProjectByID(project.ID)
	if err != nil {
		return err
	}
	start := time.Now()
	status := s.Status(p.ID)
	status.Running = true
	status.LastAttempt = start
	s.mu.Lock()
	s.statuses[p.ID] = status
	s.mu.Unlock()
	defer func() {
		status.Running = false
		status.DurationMS = time.Since(start).Milliseconds()
		if err != nil {
			status.Error = err.Error()
		} else {
			status.Error = ""
			status.LastSuccess = time.Now()
		}
		s.mu.Lock()
		s.statuses[p.ID] = status
		s.mu.Unlock()
	}()
	measured, err := usage.Measure(s.dataDir, p)
	if err != nil {
		return err
	}
	last, err := s.store.LastMetric(p.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if last == nil {
		last = &store.Metric{}
	}
	offset, identity := last.LogOffset, last.LogIdentity
	// Migrate the cursor of installations that persisted it in settings.
	if identity == "" && last.LogOffset == 0 {
		if raw, e := s.store.GetSetting(offsetKey(p.ID)); e == nil {
			offset, _ = strconv.ParseInt(raw, 10, 64)
		}
	}
	requests, bandwidth, next, newIdentity, err := readLog(filepath.Join(p.Path, ".conf", "access.log"), offset, identity)
	if err != nil {
		return err
	}
	metric := &store.Metric{ProjectID: p.ID, TS: store.Now(), DiskBytes: measured.TotalBytes,
		RequestsCum: last.RequestsCum + requests, BandwidthCum: last.BandwidthCum + bandwidth,
		LogOffset: next, LogIdentity: newIdentity}
	// Counters and their cursor are persisted together. A failed write must not
	// advance the cursor and silently discard requests.
	if err = s.store.AddMetric(metric); err != nil {
		return err
	}
	_ = s.store.DeleteSetting(offsetKey(p.ID))
	if err = s.store.UpdateProjectDisk(p.ID, measured.TotalBytes); err != nil {
		return err
	}
	status.Usage = measured
	return nil
}

var reqRe = regexp.MustCompile(`"\s+\d{3}\s+(\d+)\s+"`)

func readLog(path string, offset int64, identity string) (requests, bandwidth, next int64, newIdentity string, err error) {
	next, newIdentity = offset, identity
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, next, newIdentity, nil
	}
	if err != nil {
		return 0, 0, next, newIdentity, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return 0, 0, next, newIdentity, err
	}
	current := fileIdentity(stat)
	if offset < 0 || offset > stat.Size() || (identity != "" && current != "" && identity != current) {
		offset = 0
	}
	next, newIdentity = offset, current
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return 0, 0, next, newIdentity, err
	}
	reader := bufio.NewReaderSize(f, 64*1024)
	for {
		line, e := reader.ReadBytes('\n')
		if e != nil {
			if !errors.Is(e, io.EOF) {
				return 0, 0, offset, identity, e
			}
			break
		}
		next += int64(len(line))
		if len(line) <= 1 {
			continue
		}
		requests++
		if match := reqRe.FindSubmatch(line); len(match) == 2 {
			n, _ := strconv.ParseInt(string(match[1]), 10, 64)
			bandwidth += n
		}
	}
	return requests, bandwidth, next, newIdentity, nil
}
