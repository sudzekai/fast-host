package nginx

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ProviderLetsEncrypt = "letsencrypt"
	ProviderSelfSigned  = "selfsigned"
)

// Домены, которые не резолвятся из интернета: для них Let's Encrypt невозможен.
var localSuffixes = []string{".localhost", ".test", ".local", ".lan", ".internal", ".home.arpa", ".example", ".invalid"}

func IsLocalDomain(d string) bool {
	if d == "localhost" {
		return true
	}
	for _, s := range localSuffixes {
		if strings.HasSuffix(d, s) {
			return true
		}
	}
	return false
}

func (m *Manager) Provider(domain string) string {
	switch m.opt.SSLProvider {
	case ProviderLetsEncrypt, ProviderSelfSigned:
		return m.opt.SSLProvider
	}
	if IsLocalDomain(domain) {
		return ProviderSelfSigned
	}
	return ProviderLetsEncrypt
}

type Cert struct {
	Provider  string
	CertFile  string
	KeyFile   string
	NotBefore time.Time
	NotAfter  time.Time
}

func (m *Manager) certPaths(domain string) (provider, cert, key string) {
	provider = m.Provider(domain)
	if provider == ProviderLetsEncrypt {
		live := filepath.Join(m.opt.CertbotDir, "live", domain)
		return provider, filepath.Join(live, "fullchain.pem"), filepath.Join(live, "privkey.pem")
	}
	dir := filepath.Join(m.opt.SSLDir, domain)
	return provider, filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")
}

// CurrentCert возвращает уже выпущенный сертификат домена (если он есть на диске).
func (m *Manager) CurrentCert(domain string) (*Cert, error) {
	provider, certFile, keyFile := m.certPaths(domain)
	if _, err := os.Stat(keyFile); err != nil {
		return nil, err
	}
	leaf, err := readLeaf(certFile)
	if err != nil {
		return nil, err
	}
	return &Cert{Provider: provider, CertFile: certFile, KeyFile: keyFile, NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter}, nil
}

func readLeaf(path string) (*x509.Certificate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s: не PEM-сертификат", path)
	}
	return x509.ParseCertificate(block.Bytes)
}

// Issue выпускает (или перевыпускает) сертификат. Для Let's Encrypt сайт уже должен
// отвечать по HTTP на этом сервере: проверка идёт через /.well-known/acme-challenge/.
// email — почта владельца проекта; пустая — fallback на глобальный --certbot-email.
func (m *Manager) Issue(domain, email string) (*Cert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch m.Provider(domain) {
	case ProviderLetsEncrypt:
		if err := m.issueCertbot(domain, email); err != nil {
			return nil, err
		}
	default:
		if err := m.issueSelfSigned(domain); err != nil {
			return nil, err
		}
	}
	return m.CurrentCert(domain)
}

func (m *Manager) certbotDirs() []string {
	return []string{
		"--config-dir", m.opt.CertbotDir,
		"--work-dir", filepath.Join(m.opt.CertbotDir, "work"),
		"--logs-dir", filepath.Join(m.opt.CertbotDir, "logs"),
	}
}

func (m *Manager) issueCertbot(domain, email string) error {
	if email == "" {
		email = m.opt.CertbotEmail
	}
	if email == "" {
		return errors.New("не задан email для Let's Encrypt (--certbot-email)")
	}
	args := []string{"certonly", "--webroot", "-w", m.opt.AcmeDir,
		"--cert-name", domain, "-d", domain,
		"--non-interactive", "--agree-tos", "-m", email, "--keep-until-expiring", "--expand"}
	for _, a := range Aliases(domain)[1:] {
		// www-имя добавляем, только если оно есть в DNS: иначе Let's Encrypt откажет во всём запросе.
		if _, err := net.LookupHost(a); err == nil {
			args = append(args, "-d", a)
		}
	}
	if m.opt.CertbotStage {
		args = append(args, "--test-cert")
	}
	args = append(args, m.certbotDirs()...)
	_, err := run(m.opt.CertbotBin, args...)
	return err
}

// Самоподписанные сертификаты подписываются локальным CA (ssl/ca.pem): достаточно один раз
// добавить его в доверенные, и браузер перестанет предупреждать для всех локальных сайтов.
func (m *Manager) issueSelfSigned(domain string) error {
	caCert, caKey, err := m.localCA()
	if err != nil {
		return fmt.Errorf("локальный CA: %w", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber: randSerial(),
		Subject:      pkix.Name{CommonName: domain, Organization: []string{"FastHost"}},
		DNSNames:     Aliases(domain),
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.AddDate(0, 0, 90),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return err
	}
	dir := filepath.Join(m.opt.SSLDir, domain)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	chain := append(pemBytes("CERTIFICATE", der), pemBytes("CERTIFICATE", caCert.Raw)...)
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "privkey.pem"), pemBytes("EC PRIVATE KEY", keyDER), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "fullchain.pem"), chain, 0o644)
}

func (m *Manager) CAFile() string { return filepath.Join(m.opt.SSLDir, "ca.pem") }

func (m *Manager) localCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPath, keyPath := m.CAFile(), filepath.Join(m.opt.SSLDir, "ca-key.pem")
	if cert, err := readLeaf(certPath); err == nil {
		if b, err := os.ReadFile(keyPath); err == nil {
			if block, _ := pem.Decode(b); block != nil {
				if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil && time.Now().Before(cert.NotAfter) {
					return cert, key, nil
				}
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	tpl := &x509.Certificate{
		SerialNumber:          randSerial(),
		Subject:               pkix.Name{CommonName: "FastHost Local CA", Organization: []string{"FastHost"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(m.opt.SSLDir, 0o755); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(keyPath, pemBytes("EC PRIVATE KEY", keyDER), 0o600); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(certPath, pemBytes("CERTIFICATE", der), 0o644); err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	return cert, key, err
}

func pemBytes(typ string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}

func randSerial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	return n
}
