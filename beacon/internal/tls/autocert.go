package tls

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

type AutoTLSManager struct {
	manager  *autocert.Manager
	hostname string
	cacheDir string
}

func NewAutoTLSManager(hostname, cacheDir, email string) *AutoTLSManager {
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(cacheDir),
		HostPolicy: autocert.HostWhitelist(hostname),
		Email:      email,
	}

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

func (m *AutoTLSManager) StartChallengeServer(ctx context.Context) error {
	srv := &http.Server{
		Addr:    ":80",
		Handler: m.HTTPHandler(),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer shutdownCancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("tls challenge server error: %v", err)
		}
	}()

	return nil
}
