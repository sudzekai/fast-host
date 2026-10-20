package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hosting/internal/config"
	"hosting/internal/metrics"
	"hosting/internal/store"
)

func main() {
	cfg, err := config.FromFlags()
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.NewStorage(cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	s := metrics.New(st, cfg.DataDir, time.Duration(cfg.MetricsIntervalSec)*time.Second)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("metricsd: сбор метрик каждые %ds, драйвер БД: %s", cfg.MetricsIntervalSec, cfg.DBDriver)
	s.SampleAll()
	s.Run(ctx)
}