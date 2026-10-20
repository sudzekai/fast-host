package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"hosting/internal/auth"
	"hosting/internal/config"
	"hosting/internal/handlers"
	"hosting/internal/metrics"
	"hosting/internal/store"
)

//go:embed all:web
var webFS embed.FS

func main() {
	cfg, err := config.FromFlags()
	if err != nil {
		log.Fatal(err)
	}

	abs, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	cfg.DataDir = abs
	if err := os.MkdirAll(filepath.Join(abs, "users"), 0o755); err != nil {
		log.Fatal(err)
	}
	switch cfg.DBDriver {
	case "json":
		_ = os.MkdirAll(cfg.DBDSN, 0o755)
	case "sqlite":
		dsn := strings.TrimPrefix(cfg.DBDSN, "file:")
		if i := strings.Index(dsn, "?"); i > 0 {
			dsn = dsn[:i]
		}
		if d := filepath.Dir(dsn); d != "." {
			_ = os.MkdirAll(d, 0o755)
		}
	}

	st, err := store.NewStorage(cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	seedAdmin(st, cfg)

	var sampler *metrics.Sampler
	var cancel context.CancelFunc
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cfg.EmbedMetrics {
		sampler = metrics.New(st, cfg.DataDir, time.Duration(cfg.MetricsIntervalSec)*time.Second)
		go sampler.Run(ctx)
		go func() {
			time.Sleep(2 * time.Second)
			sampler.SampleAll()
		}()
	}

	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}
	h := handlers.New(cfg, st, sampler, sub)
	router := handlers.NewRouter(h, cfg.JWTSecret)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("FastHost запущен: http://localhost:%s (база: %s, данные: %s)", cfg.Port, cfg.DBDriver, cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("сервер: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("остановка...")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

func seedAdmin(st store.Storage, cfg *config.Config) {
	if _, err := st.GetUserByUsername(cfg.AdminUsername); err == nil {
		return
	}
	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		log.Fatal(err)
	}
	u := &store.User{
		Username:     cfg.AdminUsername,
		PasswordHash: hash,
		Role:         store.RoleAdmin,
		QuotaBytes:   store.DefaultQuotaBytes,
		CreatedAt:    time.Now().Format("2006-01-02 15:04:05"),
	}
	if err := st.CreateUser(u); err != nil {
		log.Fatalf("создание администратора: %v", err)
	}
	log.Printf("Создан администратор: %s / %s (смените пароль в панели)", cfg.AdminUsername, cfg.AdminPassword)
}