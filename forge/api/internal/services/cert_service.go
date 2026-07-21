package services

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type CertStore interface {
	CreateMTLSCertificate(ctx context.Context, req store.CreateMTLSCertificateRequest) (store.MTLSCertificate, error)
	GetMTLSCertificate(ctx context.Context, id string) (store.MTLSCertificate, error)
	ListMTLSCertificates(ctx context.Context, filter store.MTLSCertificateFilter) ([]store.MTLSCertificate, error)
	RevokeMTLSCertificate(ctx context.Context, id string) error
	GetActiveMTLSCertificateByNode(ctx context.Context, nodeID string, certType store.MTLSCertType) (store.MTLSCertificate, error)
	GetCAMTLSCertificate(ctx context.Context) (store.MTLSCertificate, error)
	GetMTLSStatus(ctx context.Context) (map[string]any, error)
}

type NodeInfoStore interface {
	GetNode(ctx context.Context, nodeID string) (store.Node, error)
	ListNodes(ctx context.Context) ([]store.Node, error)
}

type CertService struct {
	certStore CertStore
	nodeStore NodeInfoStore
	logger    *slog.Logger
}

func NewCertService(certStore CertStore, nodeStore NodeInfoStore, logger *slog.Logger) *CertService {
	return &CertService{
		certStore: certStore,
		nodeStore: nodeStore,
		logger:    logger,
	}
}

func (s *CertService) GenerateCA(ctx context.Context, org, commonName string) (store.MTLSCertificate, error) {
	if commonName == "" {
		commonName = "GamePanel mTLS CA"
	}
	if org == "" {
		org = "GamePanel"
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return store.MTLSCertificate{}, fmt.Errorf("generate serial: %w", err)
	}

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return store.MTLSCertificate{}, fmt.Errorf("generate CA key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{org},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return store.MTLSCertificate{}, fmt.Errorf("create CA certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return s.certStore.CreateMTLSCertificate(ctx, store.CreateMTLSCertificateRequest{
		CertType:       store.MTLSCertTypeCA,
		CommonName:     commonName,
		Organization:   org,
		CertificatePEM: string(certPEM),
		PrivateKey:     string(keyPEM),
		SerialNumber:   serial.String(),
		ExpiresAt:      template.NotAfter,
	})
}

func (s *CertService) GenerateCert(ctx context.Context, certType store.MTLSCertType, commonName, org, nodeID string) (store.MTLSCertificate, error) {
	ca, err := s.certStore.GetCAMTLSCertificate(ctx)
	if err != nil {
		return store.MTLSCertificate{}, fmt.Errorf("get CA certificate: %w", err)
	}
	if ca.PrivateKey == "" {
		return store.MTLSCertificate{}, fmt.Errorf("CA private key not available")
	}

	caBlock, _ := pem.Decode([]byte(ca.CertificatePEM))
	if caBlock == nil {
		return store.MTLSCertificate{}, fmt.Errorf("decode CA certificate PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return store.MTLSCertificate{}, fmt.Errorf("parse CA certificate: %w", err)
	}

	caKeyBlock, _ := pem.Decode([]byte(ca.PrivateKey))
	if caKeyBlock == nil {
		return store.MTLSCertificate{}, fmt.Errorf("decode CA private key PEM")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		caKey2, err2 := x509.ParsePKCS8PrivateKey(caKeyBlock.Bytes)
		if err2 != nil {
			return store.MTLSCertificate{}, fmt.Errorf("parse CA private key: %w", err)
		}
		var ok bool
		caKey, ok = caKey2.(*rsa.PrivateKey)
		if !ok {
			return store.MTLSCertificate{}, fmt.Errorf("CA private key is not RSA")
		}
	}

	if commonName == "" {
		commonName = uuid.NewString()
	}
	if org == "" {
		org = "GamePanel"
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return store.MTLSCertificate{}, fmt.Errorf("generate serial: %w", err)
	}

	isServer := certType == store.MTLSCertTypeServer

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{org},
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}

	if isServer {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}
	}

	var pubKey crypto.PublicKey
	var keyPEM []byte

	if isServer {
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return store.MTLSCertificate{}, fmt.Errorf("generate server key: %w", err)
		}
		pubKey = &rsaKey.PublicKey
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)})
	} else {
		ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return store.MTLSCertificate{}, fmt.Errorf("generate client key: %w", err)
		}
		pubKey = &ecKey.PublicKey
		keyDER, err := x509.MarshalPKCS8PrivateKey(ecKey)
		if err != nil {
			return store.MTLSCertificate{}, fmt.Errorf("marshal EC key: %w", err)
		}
		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	}

	var nodeIDPtr *string
	if nodeID != "" {
		nodeIDPtr = &nodeID
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, pubKey, caKey)
	if err != nil {
		return store.MTLSCertificate{}, fmt.Errorf("create %s certificate: %w", certType, err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	return s.certStore.CreateMTLSCertificate(ctx, store.CreateMTLSCertificateRequest{
		CertType:       certType,
		CommonName:     commonName,
		Organization:   org,
		CertificatePEM: string(certPEM),
		PrivateKey:     string(keyPEM),
		SerialNumber:   serial.String(),
		ExpiresAt:      template.NotAfter,
		NodeID:         nodeIDPtr,
	})
}

func (s *CertService) GetCA(ctx context.Context) (store.MTLSCertificate, error) {
	return s.certStore.GetCAMTLSCertificate(ctx)
}

func (s *CertService) GetCert(ctx context.Context, id string) (store.MTLSCertificate, error) {
	return s.certStore.GetMTLSCertificate(ctx, id)
}

func (s *CertService) ListCerts(ctx context.Context, filter store.MTLSCertificateFilter) ([]store.MTLSCertificate, error) {
	return s.certStore.ListMTLSCertificates(ctx, filter)
}

func (s *CertService) RevokeCert(ctx context.Context, id string) error {
	return s.certStore.RevokeMTLSCertificate(ctx, id)
}

func (s *CertService) GetStatus(ctx context.Context) (map[string]any, error) {
	return s.certStore.GetMTLSStatus(ctx)
}

func (s *CertService) GenerateNodeCert(ctx context.Context, nodeID string) (store.MTLSCertificate, error) {
	node, err := s.nodeStore.GetNode(ctx, nodeID)
	if err != nil {
		return store.MTLSCertificate{}, fmt.Errorf("get node: %w", err)
	}
	return s.GenerateCert(ctx, store.MTLSCertTypeClient, node.Name, "GamePanel", nodeID)
}
