package metrics

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"hosting/internal/store"
)

type failingStore struct {
	store.Storage
	fail bool
}

func (s *failingStore) AddMetric(m *store.Metric) error {
	if s.fail {
		s.fail = false
		return errors.New("test write failed")
	}
	return s.Storage.AddMetric(m)
}

func TestSamplerRetryRestartAndPartialLines(t *testing.T) {
	dir := t.TempDir()
	st, err := store.NewStorage("json", filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p := &store.Project{Path: filepath.Join(dir, "site"), Domain: "metric.test"}
	if err := os.MkdirAll(filepath.Join(p.Path, ".conf"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProject(p); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(p.Path, ".conf/access.log")
	line := `127.0.0.1 - - [10/Nov/2026:10:00:00 +0000] "GET / HTTP/1.1" 200 120 "-" "test"` + "\n"
	if err := os.WriteFile(logPath, []byte(line+"partial"), 0644); err != nil {
		t.Fatal(err)
	}
	wrapped := &failingStore{Storage: st, fail: true}
	s := New(wrapped, dir, time.Minute)
	if err := s.SampleProject(p); err == nil {
		t.Fatal("expected write failure")
	}
	if s.Status(p.ID).Error == "" {
		t.Fatal("collection failure hidden")
	}
	if err := s.SampleProject(p); err != nil {
		t.Fatal(err)
	}
	last, err := st.LastMetric(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if last.RequestsCum != 1 || last.BandwidthCum != 120 || last.LogOffset != int64(len(line)) {
		t.Fatalf("wrong counters/cursor: %+v", last)
	}
	restarted := New(st, dir, time.Minute)
	var group sync.WaitGroup
	for i := 0; i < 6; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := restarted.SampleProject(p); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	last, _ = st.LastMetric(p.ID)
	if last.RequestsCum != 1 {
		t.Fatalf("concurrent samples double counted: %d", last.RequestsCum)
	}
	if s.Status(p.ID).Error != "" || s.Status(p.ID).LastSuccess.IsZero() {
		t.Fatal("successful retry did not clear error")
	}
	if err := os.RemoveAll(p.Path); err != nil {
		t.Fatal(err)
	}
	if err := s.SampleProject(p); err == nil {
		t.Fatal("missing site reported as zero usage")
	}
	last, _ = st.LastMetric(p.ID)
	if last.RequestsCum != 1 {
		t.Fatal("failed sample changed counters")
	}
}

func TestReadLogRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	line := `host "GET / HTTP/1.1" 200 42 "-" "agent"` + "\n"
	if err := os.WriteFile(path, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, offset, identity, err := readLog(path, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if identity == "" {
		t.Skip("file identity requires Linux")
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(line+line), 0644); err != nil {
		t.Fatal(err)
	}
	requests, bandwidth, _, _, err := readLog(path, offset, identity)
	if err != nil || requests != 2 || bandwidth != 84 {
		t.Fatalf("rotation: req=%d bytes=%d err=%v", requests, bandwidth, err)
	}
}

func TestMemoryUsesAvailableAndReportsMissingData(t *testing.T) {
	m, err := parseMemory("MemTotal: 1000 kB\nMemFree: 100 kB\nMemAvailable: 400 kB\nSwapTotal: 200 kB\nSwapFree: 150 kB\n")
	if err != nil || m.UsedBytes != 600*1024 || m.AvailableBytes != 400*1024 || m.SwapUsedBytes != 50*1024 {
		t.Fatalf("memory: %+v %v", m, err)
	}
	if _, err := parseMemory("MemTotal: 1000 kB\nMemFree: 100 kB\n"); err == nil {
		t.Fatal("missing MemAvailable reported as valid")
	}
}
