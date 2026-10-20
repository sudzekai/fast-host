package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"hosting/internal/nginx"
	"hosting/internal/store"
)

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
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = persistentSecret(st)
	}

	ngx := nginx.NewManager(nginxOptions(cfg))
	if err := ngx.Init(); err != nil {
		log.Fatalf("nginx: %v", err)
	}

	var sampler *metrics.Sampler
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

	h := handlers.New(cfg, st, sampler, ngx)
	h.SyncSites()
	go h.RenewCertificates(ctx, 12*time.Hour)
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
		Email:        cfg.CertbotEmail,
		PasswordHash: hash,
		Role:         store.RoleAdmin,
		QuotaBytes:   store.DefaultQuotaBytes,
	}
	if err := st.CreateUser(u); err != nil {
		log.Fatalf("создание администратора: %v", err)
	}
	log.Printf("Создан администратор: %s (смените пароль в панели)", cfg.AdminUsername)
}

// persistentSecret генерирует секрет JWT при первом запуске и хранит его в БД,
// чтобы сессии переживали перезапуск, а секрет не был общеизвестным значением.
func persistentSecret(st store.Storage) string {
	if v, err := st.GetSetting("jwt_secret"); err == nil && len(v) >= 32 {
		return v
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	secret := hex.EncodeToString(b)
	if err := st.SetSetting("jwt_secret", secret); err != nil {
		log.Fatalf("сохранение секрета JWT: %v", err)
	}
	return secret
}

func nginxOptions(cfg *config.Config) nginx.Options {
	dataSub := func(v, name string) string {
		if v == "" {
			v = filepath.Join(cfg.DataDir, "data", name)
		}
		abs, err := filepath.Abs(v)
		if err != nil {
			log.Fatal(err)
		}
		return abs
	}
	mainConf := cfg.NginxConf
	if mainConf != "" {
		mainConf = dataSub(mainConf, "")
	}
	return nginx.Options{
		Bin:           cfg.NginxBin,
		MainConf:      mainConf,
		SitesDir:      dataSub(cfg.NginxSitesDir, filepath.Join("nginx", "sites")),
		ReloadCmd:     cfg.NginxReloadCmd,
		HTTPPort:      cfg.NginxHTTPPort,
		HTTPSPort:     cfg.NginxHTTPSPort,
		DefaultServer: cfg.NginxDefault,
		AcmeDir:       dataSub(cfg.AcmeDir, "acme"),
		SSLDir:        dataSub(cfg.SSLDir, "ssl"),
		CertbotBin:    cfg.CertbotBin,
		CertbotDir:    dataSub(cfg.CertbotDir, "letsencrypt"),
		CertbotEmail:  cfg.CertbotEmail,
		CertbotStage:  cfg.SSLStaging,
		SSLProvider:   cfg.SSLProvider,
	}
}
