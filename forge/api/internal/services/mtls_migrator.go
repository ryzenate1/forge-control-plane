package services

import (
	"context"
	"fmt"
	"log/slog"

	"gamepanel/forge/internal/store"
)

type MTLSCertificateSvc interface {
	GenerateCA(ctx context.Context, org, commonName string) (store.MTLSCertificate, error)
	GenerateNodeCert(ctx context.Context, nodeID string) (store.MTLSCertificate, error)
	GetCert(ctx context.Context, id string) (store.MTLSCertificate, error)
	ListCerts(ctx context.Context, filter store.MTLSCertificateFilter) ([]store.MTLSCertificate, error)
	RevokeCert(ctx context.Context, id string) error
	GetStatus(ctx context.Context) (map[string]any, error)
}

type MigratorStore interface {
	ListNodes(ctx context.Context) ([]store.Node, error)
	ListMTLSCertificates(ctx context.Context, filter store.MTLSCertificateFilter) ([]store.MTLSCertificate, error)
	GetCAMTLSCertificate(ctx context.Context) (store.MTLSCertificate, error)
}

type MTLSMigrator struct {
	certSvc MTLSCertificateSvc
	store   MigratorStore
	logger  *slog.Logger
}

func NewMTLSMigrator(certSvc MTLSCertificateSvc, store MigratorStore, logger *slog.Logger) *MTLSMigrator {
	return &MTLSMigrator{
		certSvc: certSvc,
		store:   store,
		logger:  logger,
	}
}

type MTLSMigrationStatus struct {
	CAConfigured     bool   `json:"caConfigured"`
	NodesWithCerts   int    `json:"nodesWithCerts"`
	TotalNodes       int    `json:"totalNodes"`
	MigrationEnabled bool   `json:"migrationEnabled"`
	Phase            string `json:"phase"`
}

func (m *MTLSMigrator) Status(ctx context.Context) (*MTLSMigrationStatus, error) {
	nodes, err := m.store.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}

	certs, err := m.store.ListMTLSCertificates(ctx, store.MTLSCertificateFilter{})
	if err != nil {
		return nil, fmt.Errorf("list certs: %w", err)
	}

	_, caErr := m.store.GetCAMTLSCertificate(ctx)
	caConfigured := caErr == nil

	nodeCertMap := make(map[string]bool)
	for _, c := range certs {
		if c.NodeID != nil {
			nodeCertMap[*c.NodeID] = true
		}
	}

	phase := "not_started"
	if caConfigured {
		phase = "ca_ready"
	}
	if len(nodeCertMap) > 0 {
		phase = "partial"
	}
	if caConfigured && len(nodeCertMap) >= len(nodes) && len(nodes) > 0 {
		phase = "complete"
	}

	return &MTLSMigrationStatus{
		CAConfigured:     caConfigured,
		NodesWithCerts:   len(nodeCertMap),
		TotalNodes:       len(nodes),
		MigrationEnabled: true,
		Phase:            phase,
	}, nil
}

func (m *MTLSMigrator) Run(ctx context.Context) error {
	m.logger.Info("starting mTLS migration")

	_, err := m.store.GetCAMTLSCertificate(ctx)
	if err != nil {
		m.logger.Info("no CA certificate found, generating one")
		if _, err := m.certSvc.GenerateCA(ctx, "GamePanel", "GamePanel mTLS CA"); err != nil {
			return fmt.Errorf("generate CA: %w", err)
		}
		m.logger.Info("CA certificate generated")
	}

	nodes, err := m.store.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	certs, err := m.store.ListMTLSCertificates(ctx, store.MTLSCertificateFilter{})
	if err != nil {
		return fmt.Errorf("list certs: %w", err)
	}

	hasCert := make(map[string]bool)
	for _, c := range certs {
		if c.NodeID != nil && c.RevokedAt == nil {
			hasCert[*c.NodeID] = true
		}
	}

	for _, node := range nodes {
		if hasCert[node.ID] {
			continue
		}
		m.logger.Info("generating certificate for node", "nodeID", node.ID, "nodeName", node.Name)
		if _, err := m.certSvc.GenerateNodeCert(ctx, node.ID); err != nil {
			return fmt.Errorf("generate cert for node %s: %w", node.ID, err)
		}
	}

	m.logger.Info("mTLS migration complete", "totalNodes", len(nodes))
	return nil
}
