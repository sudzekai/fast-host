package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	Port               string
	DBDriver           string
	DBDSN              string
	JWTSecret          string
	DataDir            string
	AdminUsername      string
	AdminPassword      string
	CertbotEmail       string
	EnableLiveSSL      bool
	AllowRegistration  bool
	MetricsIntervalSec int
	EmbedMetrics       bool
}

func FromFlags() (*Config, error) {
	cfg := &Config{
		Port:               "8080",
		DBDriver:           "json",
		DBDSN:              "data/fasthost-json",
		JWTSecret:          "fasthost-dev-secret-change-me",
		DataDir:            ".",
		AdminUsername:      "admin",
		AdminPassword:      "admin",
		CertbotEmail:       "admin@example.com",
		EnableLiveSSL:      false,
		AllowRegistration:  true,
		MetricsIntervalSec: 60,
		EmbedMetrics:       true,
	}

	port := flag.String("port", envOr("PORT", cfg.Port), "HTTP port (env PORT)")
	dbDriver := flag.String("db", envOr("DB_DRIVER", cfg.DBDriver), "database driver: json | sqlite | mysql")
	dbDSN := flag.String("dsn", "", "database DSN (env DB_DSN)")
	dataDir := flag.String("data", envOr("DATA_DIR", cfg.DataDir), "root dir (users/ subfolder created inside)")
	jwtSecret := flag.String("secret", envOr("JWT_SECRET", cfg.JWTSecret), "JWT signing secret")
	adminUser := flag.String("admin-user", envOr("ADMIN_USERNAME", cfg.AdminUsername), "initial admin login")
	adminPass := flag.String("admin-pass", envOr("ADMIN_PASSWORD", cfg.AdminPassword), "initial admin password")
	certbotEmail := flag.String("certbot-email", envOr("CERTBOT_EMAIL", cfg.CertbotEmail), "email for certbot")
	liveSSL := flag.Bool("live-ssl", envOr("ENABLE_LIVE_SSL", "0") == "1", "actually run certbot (env ENABLE_LIVE_SSL=1)")
	allowReg := flag.Bool("allow-registration", envOr("ALLOW_REGISTRATION", "1") == "1", "allow self-registration")
	noMetrics := flag.Bool("no-metrics", false, "disable embedded metrics collector")
	interval := flag.Int("metrics-interval", cfg.MetricsIntervalSec, "metrics sampling interval, seconds")
	flag.Parse()

	cfg.Port = *port
	cfg.DBDriver = *dbDriver
	if *dbDSN != "" {
		cfg.DBDSN = *dbDSN
	}
	cfg.DataDir = *dataDir
	cfg.JWTSecret = *jwtSecret
	cfg.AdminUsername = *adminUser
	cfg.AdminPassword = *adminPass
	cfg.CertbotEmail = *certbotEmail
	cfg.EnableLiveSSL = *liveSSL
	cfg.AllowRegistration = *allowReg
	cfg.EmbedMetrics = !*noMetrics
	cfg.MetricsIntervalSec = *interval

	switch cfg.DBDriver {
	case "json":
		if cfg.DBDSN == "" {
			cfg.DBDSN = "data/fasthost-json"
		}
	case "sqlite":
		if cfg.DBDSN == "" {
			cfg.DBDSN = "data/fasthost.db"
		}
	case "mysql":
		if cfg.DBDSN == "" {
			cfg.DBDSN = "fasthost:fasthost@tcp(127.0.0.1:3306)/fasthost?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&loc=Local"
		}
	default:
		return nil, fmt.Errorf("unknown db driver %q (use json | sqlite | mysql)", cfg.DBDriver)
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}