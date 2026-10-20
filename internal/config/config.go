package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port               string
	DBDriver           string
	DBDSN              string
	JWTSecret          string
	DataDir            string
	AdminUsername      string
	AdminPassword      string
	AllowRegistration  bool
	MetricsIntervalSec int
	EmbedMetrics       bool
	CORSOrigins        []string

	NginxBin       string
	NginxConf      string // пусто — системный nginx; иначе панель сама ведёт главный конфиг и процесс
	NginxSitesDir  string
	NginxReloadCmd string
	NginxHTTPPort  int
	NginxHTTPSPort int
	NginxDefault   bool

	SSLProvider  string
	SSLDir       string
	AcmeDir      string
	CertbotBin   string
	CertbotDir   string
	CertbotEmail string
	SSLStaging   bool
}

func FromFlags() (*Config, error) {
	cfg := &Config{
		Port:               "8080",
		DBDriver:           "json",
		DBDSN:              "data/fasthost-json",
		JWTSecret:          "",
		DataDir:            ".",
		AdminUsername:      "admin",
		AdminPassword:      "admin",
		AllowRegistration:  true,
		MetricsIntervalSec: 60,
		EmbedMetrics:       true,
	}

	port := flag.String("port", envOr("PORT", cfg.Port), "HTTP port (env PORT)")
	dbDriver := flag.String("db", envOr("DB_DRIVER", cfg.DBDriver), "database driver: json | sqlite | mysql")
	dbDSN := flag.String("dsn", envOr("DB_DSN", ""), "database DSN (env DB_DSN)")
	dataDir := flag.String("data", envOr("DATA_DIR", cfg.DataDir), "root dir (users/ subfolder created inside)")
	jwtSecret := flag.String("secret", envOr("JWT_SECRET", ""), "JWT signing secret (empty: random, stored in DB)")
	adminUser := flag.String("admin-user", envOr("ADMIN_USERNAME", cfg.AdminUsername), "initial admin login")
	adminPass := flag.String("admin-pass", envOr("ADMIN_PASSWORD", cfg.AdminPassword), "initial admin password")
	allowReg := flag.Bool("allow-registration", envOr("ALLOW_REGISTRATION", "1") == "1", "allow self-registration")
	noMetrics := flag.Bool("no-metrics", false, "disable embedded metrics collector")
	interval := flag.Int("metrics-interval", cfg.MetricsIntervalSec, "metrics sampling interval, seconds")
	corsOrigins := flag.String("cors-origins", envOr("CORS_ORIGINS", ""), "comma-separated allowed browser origins (empty: same-origin only)")

	nginxBin := flag.String("nginx-bin", envOr("NGINX_BIN", "nginx"), "nginx executable")
	nginxConf := flag.String("nginx-conf", envOr("NGINX_CONF", ""), "own main nginx.conf managed by the panel (empty: system nginx)")
	nginxSites := flag.String("nginx-sites", envOr("NGINX_SITES_DIR", ""), "dir with site configs included by nginx (default {data}/data/nginx/sites)")
	nginxReload := flag.String("nginx-reload-cmd", envOr("NGINX_RELOAD_CMD", ""), "custom reload command, e.g. \"sudo systemctl reload nginx\"")
	httpPort := flag.Int("nginx-http-port", envInt("NGINX_HTTP_PORT", 80), "port nginx listens for HTTP")
	httpsPort := flag.Int("nginx-https-port", envInt("NGINX_HTTPS_PORT", 443), "port nginx listens for HTTPS")
	nginxDefault := flag.Bool("nginx-default-server", envOr("NGINX_DEFAULT_SERVER", "1") == "1", "add catch-all default_server for unknown/disabled domains")

	sslProvider := flag.String("ssl-provider", envOr("SSL_PROVIDER", "auto"), "auto | letsencrypt | selfsigned (auto: selfsigned for *.localhost, *.test, ...)")
	sslDir := flag.String("ssl-dir", envOr("SSL_DIR", ""), "dir for self-signed certs and local CA (default {data}/data/ssl)")
	acmeDir := flag.String("acme-dir", envOr("ACME_DIR", ""), "webroot for ACME challenges (default {data}/data/acme)")
	certbotBin := flag.String("certbot-bin", envOr("CERTBOT_BIN", "certbot"), "certbot executable")
	certbotDir := flag.String("certbot-dir", envOr("CERTBOT_DIR", ""), "certbot --config-dir (default {data}/data/letsencrypt)")
	certbotEmail := flag.String("certbot-email", envOr("CERTBOT_EMAIL", "admin@example.com"), "email for Let's Encrypt")
	sslStaging := flag.Bool("ssl-staging", envOr("SSL_STAGING", "0") == "1", "use Let's Encrypt staging (test certificates)")
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
	cfg.AllowRegistration = *allowReg
	cfg.EmbedMetrics = !*noMetrics
	cfg.MetricsIntervalSec = *interval
	for _, o := range strings.Split(*corsOrigins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.CORSOrigins = append(cfg.CORSOrigins, o)
		}
	}

	cfg.NginxBin = *nginxBin
	cfg.NginxConf = *nginxConf
	cfg.NginxSitesDir = *nginxSites
	cfg.NginxReloadCmd = *nginxReload
	cfg.NginxHTTPPort = *httpPort
	cfg.NginxHTTPSPort = *httpsPort
	cfg.NginxDefault = *nginxDefault
	cfg.SSLProvider = *sslProvider
	cfg.SSLDir = *sslDir
	cfg.AcmeDir = *acmeDir
	cfg.CertbotBin = *certbotBin
	cfg.CertbotDir = *certbotDir
	cfg.CertbotEmail = *certbotEmail
	cfg.SSLStaging = *sslStaging

	switch cfg.SSLProvider {
	case "auto", "letsencrypt", "selfsigned":
	default:
		return nil, fmt.Errorf("unknown ssl provider %q (use auto | letsencrypt | selfsigned)", cfg.SSLProvider)
	}

	switch cfg.DBDriver {
	case "json":
		if *dbDSN == "" {
			cfg.DBDSN = "data/fasthost-json"
		}
	case "sqlite":
		if *dbDSN == "" {
			cfg.DBDSN = "data/fasthost.db"
		}
	case "mysql":
		if *dbDSN == "" {
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

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return n
	}
	return def
}
