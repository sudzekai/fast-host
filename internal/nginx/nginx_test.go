package nginx

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testManager(t *testing.T) *Manager {
	dir := t.TempDir()
	return NewManager(Options{
		SitesDir: filepath.Join(dir, "sites"), AcmeDir: filepath.Join(dir, "acme"),
		SSLDir: filepath.Join(dir, "ssl"), CertbotDir: filepath.Join(dir, "le"),
	})
}

func TestRenderHasNoHumanComments(t *testing.T) {
	m := testManager(t)
	for _, ssl := range []bool{false, true} {
		cfg := m.Render(Site{Domain: "example.com", Root: "/srv/site", LogDir: "/srv/site/.conf", SSL: ssl, CertFile: "/c.pem", KeyFile: "/k.pem"})
		for _, line := range strings.Split(cfg, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				t.Fatalf("ssl=%v: comment in generated config: %q", ssl, line)
			}
		}
		if strings.Contains(cfg, "sudo") {
			t.Fatalf("ssl=%v: instructions leaked into config", ssl)
		}
	}
}

func TestRenderSSLRedirects(t *testing.T) {
	m := testManager(t)
	cfg := m.Render(Site{Domain: "example.com", Root: "/srv/site", LogDir: "/srv/site/.conf", SSL: true, CertFile: "/c.pem", KeyFile: "/k.pem"})
	for _, want := range []string{"return 301 https://$host$request_uri;", "listen 443 ssl;", "ssl_certificate /c.pem;", "/.well-known/acme-challenge/"} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("missing %q in:\n%s", want, cfg)
		}
	}
	plain := m.Render(Site{Domain: "example.com", Root: "/srv/site", LogDir: "/srv/site/.conf"})
	if strings.Contains(plain, "return 301") || strings.Contains(plain, "443") {
		t.Fatalf("non-SSL config must not redirect:\n%s", plain)
	}
}

func TestSelfSignedIssue(t *testing.T) {
	m := testManager(t)
	if got := m.Provider("demo.localhost"); got != ProviderSelfSigned {
		t.Fatalf("provider for local domain: %s", got)
	}
	if got := m.Provider("example.com"); got != ProviderLetsEncrypt {
		t.Fatalf("provider for public domain: %s", got)
	}
	c, err := m.Issue("demo.localhost", "")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(c.NotAfter); d < 89*24*time.Hour || d > 91*24*time.Hour {
		t.Fatalf("unexpected validity: %v", d)
	}
	leaf, err := readLeaf(c.CertFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := leaf.VerifyHostname("www.demo.localhost"); err != nil {
		t.Fatal(err)
	}
}
