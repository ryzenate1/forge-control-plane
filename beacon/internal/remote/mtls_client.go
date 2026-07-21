package remote

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type MTLSClientConfig struct {
	Enabled     bool
	PanelURL    string
	Token       string
	CACertPath  string
	CertPath    string
	KeyPath     string
}

type MTLSClient struct {
	inner    Client
	config   MTLSClientConfig
	mu       sync.RWMutex
	httpCli  *http.Client
}

func NewMTLSClient(config MTLSClientConfig) Client {
	if !config.Enabled {
		return NewClient(config.PanelURL, config.Token)
	}

	tlsConfig, err := buildMTLSTLSConfig(config)
	if err != nil {
		log.Printf("[mtls] failed to build TLS config, falling back to token auth: %v", err)
		return NewClient(config.PanelURL, config.Token)
	}

	base := NewClient(config.PanelURL, config.Token).(*client)
	base.httpClient = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	return &MTLSClient{
		inner:  base,
		config: config,
		httpCli: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
	}
}

func buildMTLSTLSConfig(config MTLSClientConfig) (*tls.Config, error) {
	caData, err := os.ReadFile(config.CACertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("no CA certificates found in PEM data")
	}

	certData, err := os.ReadFile(config.CertPath)
	if err != nil {
		return nil, fmt.Errorf("read client cert: %w", err)
	}

	keyData, err := os.ReadFile(config.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("read client key: %w", err)
	}

	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return nil, fmt.Errorf("parse client key pair: %w", err)
	}

	return &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cert},
		ServerName:   "", // derived from panel URL
		MinVersion:   tls.VersionTLS12,
	}, nil
}

func (m *MTLSClient) Inner() Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.inner
}

func (m *MTLSClient) GetServerConfiguration(ctx context.Context, uuid string) (ServerConfigurationResponse, error) {
	return m.Inner().GetServerConfiguration(ctx, uuid)
}

func (m *MTLSClient) GetServers(ctx context.Context, perPage int) ([]RawServerData, error) {
	return m.Inner().GetServers(ctx, perPage)
}

func (m *MTLSClient) ResetServersState(ctx context.Context) error {
	return m.Inner().ResetServersState(ctx)
}

func (m *MTLSClient) SendActivityLogs(ctx context.Context, activity []Activity) error {
	return m.Inner().SendActivityLogs(ctx, activity)
}

func (m *MTLSClient) SendServerStats(ctx context.Context, serverID string, stats ServerStats) error {
	return m.Inner().SendServerStats(ctx, serverID, stats)
}

func (m *MTLSClient) SendNodeHeartbeat(ctx context.Context, nodeID string, heartbeat NodeHeartbeat) error {
	return m.Inner().SendNodeHeartbeat(ctx, nodeID, heartbeat)
}

func (m *MTLSClient) CreatePlacementReservation(ctx context.Context, req PlacementReservationRequest) (PlacementReservation, error) {
	return m.Inner().CreatePlacementReservation(ctx, req)
}

func (m *MTLSClient) ConfirmPlacementReservation(ctx context.Context, reservationID string) error {
	return m.Inner().ConfirmPlacementReservation(ctx, reservationID)
}

func (m *MTLSClient) CancelPlacementReservation(ctx context.Context, reservationID string) error {
	return m.Inner().CancelPlacementReservation(ctx, reservationID)
}

func (m *MTLSClient) TriggerServerBackup(ctx context.Context, serverID string) error {
	return m.Inner().TriggerServerBackup(ctx, serverID)
}

func (m *MTLSClient) ReportEvacuationProgress(ctx context.Context, evacuationID string, progress EvacuationProgress) error {
	return m.Inner().ReportEvacuationProgress(ctx, evacuationID, progress)
}

func (m *MTLSClient) SetInstallationStatus(ctx context.Context, serverID string, successful bool) error {
	return m.Inner().SetInstallationStatus(ctx, serverID, successful)
}

func (m *MTLSClient) SendCrashEvent(ctx context.Context, serverID string, exitCode int, oomKilled bool, autoRestart bool) error {
	return m.Inner().SendCrashEvent(ctx, serverID, exitCode, oomKilled, autoRestart)
}

func (m *MTLSClient) SendBackupStatus(ctx context.Context, serverID string, req BackupStatusRequest) error {
	return m.Inner().SendBackupStatus(ctx, serverID, req)
}

func (m *MTLSClient) SendRestoreStatus(ctx context.Context, serverID string, req RestoreStatusRequest) error {
	return m.Inner().SendRestoreStatus(ctx, serverID, req)
}

func (m *MTLSClient) SendCapabilityReport(ctx context.Context, report interface{}) error {
	return m.Inner().SendCapabilityReport(ctx, report)
}

func (m *MTLSClient) ReloadCertificates() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tlsConfig, err := buildMTLSTLSConfig(m.config)
	if err != nil {
		return fmt.Errorf("reload TLS config: %w", err)
	}

	m.httpCli = &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	base := NewClient(m.config.PanelURL, m.config.Token).(*client)
	base.httpClient = m.httpCli
	m.inner = base

	return nil
}
