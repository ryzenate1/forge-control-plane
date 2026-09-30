package trafficmanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testCertPEM = "-----BEGIN CERTIFICATE-----\nMIIBtest\n-----END CERTIFICATE-----\n"
	testKeyPEM  = "-----BEGIN PRIVATE KEY-----\nMIIBkeytest\n-----END PRIVATE KEY-----\n"
)

// newTestProxy builds a Traefik proxy writing into a temp dir, with no reload
// endpoint configured so reloadTraefik stays a local no-op.
func newTestProxy(t *testing.T) *TraefikReverseProxy {
	t.Helper()
	return &TraefikReverseProxy{configDir: t.TempDir()}
}

// TestSetCertificateWritesPEMToDisk is the regression guard for certificate
// bodies being assigned into Traefik's certFile/keyFile path fields, which
// made Traefik try to open a file named after the PEM header.
func TestSetCertificateWritesPEMToDisk(t *testing.T) {
	p := newTestProxy(t)

	err := p.SetCertificate(context.Background(), CertConfig{
		Certificate: testCertPEM,
		PrivateKey:  testKeyPEM,
		Domains:     []string{"example.com"},
	})
	if err != nil {
		t.Fatalf("SetCertificate: %v", err)
	}

	tlsCfg := p.loadTLSConfig(filepath.Join(p.configDir, "tls.yml"))
	if len(tlsCfg.TLS) != 1 {
		t.Fatalf("expected exactly one TLS entry, got %d", len(tlsCfg.TLS))
	}
	entry := tlsCfg.TLS[0]

	if strings.Contains(entry.CertFile, "BEGIN CERTIFICATE") {
		t.Fatal("certFile holds PEM contents instead of a path")
	}
	if strings.Contains(entry.KeyFile, "BEGIN PRIVATE KEY") {
		t.Fatal("keyFile holds PEM contents instead of a path")
	}

	onDisk, err := os.ReadFile(entry.CertFile)
	if err != nil {
		t.Fatalf("certFile does not point at a readable file: %v", err)
	}
	if string(onDisk) != testCertPEM {
		t.Error("certificate on disk does not match what was supplied")
	}

	keyOnDisk, err := os.ReadFile(entry.KeyFile)
	if err != nil {
		t.Fatalf("keyFile does not point at a readable file: %v", err)
	}
	if string(keyOnDisk) != testKeyPEM {
		t.Error("private key on disk does not match what was supplied")
	}

	info, err := os.Stat(entry.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("private key should be 0600, got %o", perm)
	}
}

// TestSetCertificateIsIdempotent guards the loop that appended one identical
// entry per domain on every call.
func TestSetCertificateIsIdempotent(t *testing.T) {
	p := newTestProxy(t)
	cert := CertConfig{
		Certificate: testCertPEM,
		PrivateKey:  testKeyPEM,
		Domains:     []string{"example.com", "www.example.com"},
	}

	for i := 0; i < 3; i++ {
		if err := p.SetCertificate(context.Background(), cert); err != nil {
			t.Fatalf("SetCertificate call %d: %v", i+1, err)
		}
	}

	tlsCfg := p.loadTLSConfig(filepath.Join(p.configDir, "tls.yml"))
	if len(tlsCfg.TLS) != 2 {
		t.Fatalf("expected one entry per domain after repeated renewals, got %d", len(tlsCfg.TLS))
	}
}

// TestRemoveCertificateHandlesWildcards guards removal for domains whose
// filenames cannot contain the literal domain string.
func TestRemoveCertificateHandlesWildcards(t *testing.T) {
	p := newTestProxy(t)
	if err := p.SetCertificate(context.Background(), CertConfig{
		Certificate: testCertPEM,
		PrivateKey:  testKeyPEM,
		Domains:     []string{"*.example.com", "other.test"},
	}); err != nil {
		t.Fatalf("SetCertificate: %v", err)
	}

	if err := p.RemoveCertificate(context.Background(), []string{"*.example.com"}); err != nil {
		t.Fatalf("RemoveCertificate: %v", err)
	}

	tlsCfg := p.loadTLSConfig(filepath.Join(p.configDir, "tls.yml"))
	if len(tlsCfg.TLS) != 1 {
		t.Fatalf("expected the wildcard entry to be dropped, got %d entries", len(tlsCfg.TLS))
	}
	if !strings.Contains(tlsCfg.TLS[0].CertFile, "other.test") {
		t.Errorf("wrong entry survived removal: %s", tlsCfg.TLS[0].CertFile)
	}

	wildcardCert, _ := p.certFilePaths("*.example.com")
	if _, err := os.Stat(wildcardCert); !os.IsNotExist(err) {
		t.Error("removed certificate should be deleted from disk")
	}
}

// TestSetCertificateRejectsEmptyMaterial ensures a blank certificate is a
// failure rather than an empty file Traefik would silently reject.
func TestSetCertificateRejectsEmptyMaterial(t *testing.T) {
	p := newTestProxy(t)
	for _, tc := range []struct {
		name string
		cert CertConfig
	}{
		{"no certificate", CertConfig{PrivateKey: testKeyPEM, Domains: []string{"a.test"}}},
		{"no key", CertConfig{Certificate: testCertPEM, Domains: []string{"a.test"}}},
		{"no domains", CertConfig{Certificate: testCertPEM, PrivateKey: testKeyPEM}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := p.SetCertificate(context.Background(), tc.cert); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
