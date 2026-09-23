package daemon

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// NOTE: daemonMTLSConfig (client-certificate mTLS for the daemon HTTP client)
// was removed from production code: the daemon client now authenticates with
// HMAC request signing only, and daemonTransport() enforces TLS 1.2+ without
// loading any MTLS_* certificate material. The tests below assert that
// current behavior; the TLS handshake integration test is kept because it
// still documents how a client-cert-enforcing server interacts with plain
// HMAC-only clients.

func generateCATest(t testing.TB) ([]byte, []byte, *x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "TestCA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour * 24),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}
	return certPEM, keyPEM, cert, key
}

func generateLeafTest(t testing.TB, caCert *x509.Certificate, caKey *rsa.PrivateKey, commonName string, isClient bool, hosts []string) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour * 24),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	if isClient {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.DNSNames = hosts
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

func writeTempFile(t testing.TB, dir, name string, data []byte, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestDaemonTransport_TLSMinVersionOnly(t *testing.T) {
	_ = writeTempFile // keep helper referenced
	tr := daemonTransport()
	if tr.TLSClientConfig == nil {
		t.Fatal("expected TLSClientConfig")
	}
	if got := tr.TLSClientConfig.MinVersion; got != uint16(tls.VersionTLS12) {
		t.Fatalf("expected TLS 1.2 minimum, got %d", got)
	}
	if tr.TLSClientConfig.RootCAs != nil || len(tr.TLSClientConfig.Certificates) != 0 {
		t.Fatal("daemonTransport must not load client certs (mTLS removed)")
	}
}

func TestDaemonTransport_IgnoresMTLSEnv(t *testing.T) {
	// Even with MTLS_* env vars pointing at missing files, the transport must
	// build cleanly: the daemon client no longer consumes them.
	dir := t.TempDir()
	t.Setenv("MTLS_ENABLED", "true")
	t.Setenv("MTLS_CA_CERT", filepath.Join(dir, "missing-ca.pem"))
	t.Setenv("MTLS_CERT", filepath.Join(dir, "missing.pem"))
	t.Setenv("MTLS_KEY", filepath.Join(dir, "missing.key"))

	tr := daemonTransport()
	if tr == nil {
		t.Fatal("expected transport")
	}
	if len(tr.TLSClientConfig.Certificates) != 0 {
		t.Fatal("expected no client certificates regardless of env")
	}
}

// TestDaemonMTLSIntegration_ValidCertPasses validates that a valid client cert
// is presented and the TLS handshake succeeds on a server that requires client
// certificates, while an HMAC-only client (as the daemon now is) is rejected
// by such a server.
func TestDaemonMTLSIntegration_ValidCertPasses(t *testing.T) {
	_ = t.TempDir()
	caPEM, _, caCert, caKey := generateCATest(t)
	serverCertPEM, serverKeyPEM := generateLeafTest(t, caCert, caKey, "beacon", false, []string{"localhost"})
	clientCertPEM, clientKeyPEM := generateLeafTest(t, caCert, caKey, "panel", true, nil)
	// also generate a bad CA/client for negative test
	badCAPEM, _, badCACert, badCAKey := generateCATest(t)
	badClientCertPEM, badClientKeyPEM := generateLeafTest(t, badCACert, badCAKey, "panel-bad", true, nil)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	// Server TLS config requiring client cert
	serverCert, _ := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS12,
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Also verify HMAC-like header present for defense-in-depth check
		if r.Header.Get("X-Panel-Signature") == "" {
			http.Error(w, "missing HMAC", http.StatusUnauthorized)
			return
		}
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "missing client cert", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	// Helper to build a client TLS config similar to the removed daemonMTLSConfig but for test
	buildClientTLS := func(caPEM, certPEM, keyPEM []byte) *tls.Config {
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caPEM)
		cert, _ := tls.X509KeyPair(certPEM, keyPEM)
		return &tls.Config{
			RootCAs:      pool,
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			ServerName:   "localhost",
		}
	}

	// Positive: valid cert + HMAC passes
	t.Run("valid cert passes", func(t *testing.T) {
		tlsCfg := buildClientTLS(caPEM, clientCertPEM, clientKeyPEM)
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		req.Header.Set("X-Panel-Signature", "dummy-hmac-for-test")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("valid cert request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("valid cert status=%d, want 200", resp.StatusCode)
		}
	})

	// Negative: invalid cert (signed by different CA) fails handshake
	t.Run("invalid cert fails", func(t *testing.T) {
		_ = badCAPEM
		tlsCfg := buildClientTLS(caPEM, badClientCertPEM, badClientKeyPEM)
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		req.Header.Set("X-Panel-Signature", "dummy")
		_, err := client.Do(req)
		if err == nil {
			t.Fatal("expected handshake failure with invalid client cert")
		}
	})

	// Negative: missing cert fails (no Certificates)
	t.Run("missing cert fails", func(t *testing.T) {
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caPEM)
		tlsCfg := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12, ServerName: "localhost"}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		req.Header.Set("X-Panel-Signature", "dummy")
		_, err := client.Do(req)
		if err == nil {
			t.Fatal("expected failure with missing client cert")
		}
	})

	// When mTLS disabled, HMAC-only passes (no client cert required)
	t.Run("HMAC without mTLS passes when disabled", func(t *testing.T) {
		// Server without ClientAuth
		noMTLSServerTLS := &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientAuth:   tls.NoClientCert,
			MinVersion:   tls.VersionTLS12,
		}
		handler2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Panel-Signature") == "" {
				http.Error(w, "missing HMAC", http.StatusUnauthorized)
				return
			}
			// mTLS disabled: do not check PeerCertificates
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		})
		srv2 := httptest.NewUnstartedServer(handler2)
		srv2.TLS = noMTLSServerTLS
		srv2.StartTLS()
		defer srv2.Close()
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caPEM)
		tlsCfg := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12, ServerName: "localhost"}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}
		req, _ := http.NewRequest(http.MethodGet, srv2.URL, nil)
		req.Header.Set("X-Panel-Signature", "dummy-hmac")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("HMAC-only request failed when mTLS disabled: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("HMAC-only status=%d, want 200", resp.StatusCode)
		}
	})
}

func TestDaemonClient_NewClient_SucceedsDespiteMTLSEnv(t *testing.T) {
	// mTLS misconfiguration can no longer break NewClient: the client never
	// reads MTLS_* variables. Non-loopback URLs are upgraded to https and the
	// transport stays cert-free.
	url := "https://beacon.example.com:9090"
	t.Setenv("MTLS_ENABLED", "true")
	t.Setenv("MTLS_CERT", "/tmp/missing.pem")
	t.Setenv("MTLS_KEY", "/tmp/missing.key")
	t.Setenv("MTLS_CA_CERT", "/tmp/missing-ca.pem")
	cli, err := NewClient(url, "node.secret")
	if err != nil {
		t.Fatalf("NewClient should ignore MTLS_* env: %v", err)
	}
	if cli == nil {
		t.Fatal("expected client")
	}
}
