package tls

import (
	"context"
	"crypto/tls"
	"net/http"

	"golang.org/x/crypto/acme/autocert"
)

type AutoTLSManager struct {
	manager  *autocert.Manager
	hostname string
	cacheDir string
}

func NewAutoTLSManager(hostname, cacheDir, email string) *AutoTLSManager {
	var emailContact []string
	if email != "" {
		emailContact = []string{"mailto:" + email}
	}

	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(cacheDir),
		HostPolicy: autocert.HostWhitelist(hostname),
		Email:      email,
	}
	_ = emailContact

	return &AutoTLSManager{
		manager:  m,
		hostname: hostname,
		cacheDir: cacheDir,
	}
}

func (m *AutoTLSManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return m.manager.GetCertificate(hello)
}

func (m *AutoTLSManager) HTTPHandler() http.Handler {
	return m.manager.HTTPHandler(nil)
}

func (m *AutoTLSManager) StartChallengeServer() error {
	srv := &http.Server{
		Addr:    ":80",
		Handler: m.HTTPHandler(),
	}

	go func() {
		_ = srv.ListenAndServe()
	}()

	go func() {
		<-context.Background().Done()
		_ = srv.Close()
	}()

	return nil
}
