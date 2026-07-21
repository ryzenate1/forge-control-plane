package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type MTLSCertType string

const (
	MTLSCertTypeCA     MTLSCertType = "ca"
	MTLSCertTypeServer MTLSCertType = "server"
	MTLSCertTypeClient MTLSCertType = "client"
)

type MTLSCertificate struct {
	ID                 string       `json:"id"`
	CertType           MTLSCertType `json:"certType"`
	CommonName         string       `json:"commonName"`
	Organization       string       `json:"organization"`
	CertificatePEM     string       `json:"certificatePem"`
	PrivateKey         string       `json:"-"`
	SerialNumber       string       `json:"serialNumber"`
	ExpiresAt          time.Time    `json:"expiresAt"`
	RevokedAt          *time.Time   `json:"revokedAt,omitempty"`
	NodeID             *string      `json:"nodeId,omitempty"`
	CreatedAt          time.Time    `json:"createdAt"`
}

type CreateMTLSCertificateRequest struct {
	CertType       MTLSCertType
	CommonName     string
	Organization   string
	CertificatePEM string
	PrivateKey     string
	SerialNumber   string
	ExpiresAt      time.Time
	NodeID         *string
}

type MTLSCertificateFilter struct {
	CertType *MTLSCertType
	NodeID   *string
	Revoked  *bool
	Limit    int
	Offset   int
}

func (s *Store) CreateMTLSCertificate(ctx context.Context, req CreateMTLSCertificateRequest) (MTLSCertificate, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	encryptedKey, err := s.encryptSecret(req.PrivateKey, secretAAD("mtls_certificates", id, "private_key"))
	if err != nil {
		return MTLSCertificate{}, err
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO mtls_certificates (id, cert_type, common_name, organization, certificate_pem, private_key_encrypted, serial_number, expires_at, node_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, id, string(req.CertType), req.CommonName, req.Organization, req.CertificatePEM, encryptedKey, req.SerialNumber, req.ExpiresAt, req.NodeID, now)
	if err != nil {
		return MTLSCertificate{}, err
	}

	return MTLSCertificate{
		ID:             id,
		CertType:       req.CertType,
		CommonName:     req.CommonName,
		Organization:   req.Organization,
		CertificatePEM: req.CertificatePEM,
		PrivateKey:     req.PrivateKey,
		SerialNumber:   req.SerialNumber,
		ExpiresAt:      req.ExpiresAt,
		NodeID:         req.NodeID,
		CreatedAt:      now,
	}, nil
}

func (s *Store) GetMTLSCertificate(ctx context.Context, id string) (MTLSCertificate, error) {
	var cert MTLSCertificate
	var keyEncrypted string
	var certTypeStr string

	err := s.db.QueryRow(ctx, `
		SELECT id::text, cert_type::text, COALESCE(common_name,''), COALESCE(organization,''),
		       COALESCE(certificate_pem,''), COALESCE(private_key_encrypted,''),
		       COALESCE(serial_number,''), expires_at, revoked_at, node_id::text, created_at
		FROM mtls_certificates WHERE id::text = $1
	`, id).Scan(&cert.ID, &certTypeStr, &cert.CommonName, &cert.Organization,
		&cert.CertificatePEM, &keyEncrypted, &cert.SerialNumber,
		&cert.ExpiresAt, &cert.RevokedAt, &cert.NodeID, &cert.CreatedAt)
	if err != nil {
		return MTLSCertificate{}, errors.New("mtls certificate not found")
	}

	cert.CertType = MTLSCertType(certTypeStr)

	if keyEncrypted != "" && s.secrets != nil {
		decrypted, decErr := s.decryptSecret(keyEncrypted, "", secretAAD("mtls_certificates", cert.ID, "private_key"))
		if decErr == nil {
			cert.PrivateKey = decrypted
		}
	}

	return cert, nil
}

func (s *Store) ListMTLSCertificates(ctx context.Context, filter MTLSCertificateFilter) ([]MTLSCertificate, error) {
	query := `
		SELECT id::text, cert_type::text, COALESCE(common_name,''), COALESCE(organization,''),
		       COALESCE(certificate_pem,''), '', COALESCE(serial_number,''),
		       expires_at, revoked_at, node_id::text, created_at
		FROM mtls_certificates WHERE 1=1`
	args := []any{}
	argIdx := 1

	if filter.CertType != nil {
		query += " AND cert_type = $" + itoa(argIdx)
		args = append(args, string(*filter.CertType))
		argIdx++
	}
	if filter.NodeID != nil {
		query += " AND node_id::text = $" + itoa(argIdx)
		args = append(args, *filter.NodeID)
		argIdx++
	}
	if filter.Revoked != nil {
		if *filter.Revoked {
			query += " AND revoked_at IS NOT NULL"
		} else {
			query += " AND revoked_at IS NULL"
		}
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT $" + itoa(argIdx)
		args = append(args, filter.Limit)
		argIdx++
	}
	if filter.Offset > 0 {
		query += " OFFSET $" + itoa(argIdx)
		args = append(args, filter.Offset)
		argIdx++
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []MTLSCertificate
	for rows.Next() {
		var cert MTLSCertificate
		var certTypeStr string
		var _pk string
		if err := rows.Scan(&cert.ID, &certTypeStr, &cert.CommonName, &cert.Organization,
			&cert.CertificatePEM, &_pk, &cert.SerialNumber,
			&cert.ExpiresAt, &cert.RevokedAt, &cert.NodeID, &cert.CreatedAt); err != nil {
			return nil, err
		}
		cert.CertType = MTLSCertType(certTypeStr)
		certs = append(certs, cert)
	}
	return certs, rows.Err()
}

func (s *Store) RevokeMTLSCertificate(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `UPDATE mtls_certificates SET revoked_at = $1 WHERE id::text = $2 AND revoked_at IS NULL`, now, id)
	return err
}

func (s *Store) GetActiveMTLSCertificateByNode(ctx context.Context, nodeID string, certType MTLSCertType) (MTLSCertificate, error) {
	var cert MTLSCertificate
	var keyEncrypted string
	var certTypeStr string

	err := s.db.QueryRow(ctx, `
		SELECT id::text, cert_type::text, COALESCE(common_name,''), COALESCE(organization,''),
		       COALESCE(certificate_pem,''), COALESCE(private_key_encrypted,''),
		       COALESCE(serial_number,''), expires_at, revoked_at, node_id::text, created_at
		FROM mtls_certificates
		WHERE node_id::text = $1 AND cert_type = $2 AND revoked_at IS NULL AND expires_at > NOW()
		ORDER BY created_at DESC LIMIT 1
	`, nodeID, string(certType)).Scan(&cert.ID, &certTypeStr, &cert.CommonName, &cert.Organization,
		&cert.CertificatePEM, &keyEncrypted, &cert.SerialNumber,
		&cert.ExpiresAt, &cert.RevokedAt, &cert.NodeID, &cert.CreatedAt)
	if err != nil {
		return MTLSCertificate{}, errors.New("no active mtls certificate found for node")
	}

	cert.CertType = MTLSCertType(certTypeStr)

	if keyEncrypted != "" && s.secrets != nil {
		decrypted, decErr := s.decryptSecret(keyEncrypted, "", secretAAD("mtls_certificates", cert.ID, "private_key"))
		if decErr == nil {
			cert.PrivateKey = decrypted
		}
	}

	return cert, nil
}

func (s *Store) GetCAMTLSCertificate(ctx context.Context) (MTLSCertificate, error) {
	var cert MTLSCertificate
	var keyEncrypted string
	var certTypeStr string

	err := s.db.QueryRow(ctx, `
		SELECT id::text, cert_type::text, COALESCE(common_name,''), COALESCE(organization,''),
		       COALESCE(certificate_pem,''), COALESCE(private_key_encrypted,''),
		       COALESCE(serial_number,''), expires_at, revoked_at, node_id::text, created_at
		FROM mtls_certificates
		WHERE cert_type = 'ca' AND revoked_at IS NULL AND expires_at > NOW()
		ORDER BY created_at DESC LIMIT 1
	`).Scan(&cert.ID, &certTypeStr, &cert.CommonName, &cert.Organization,
		&cert.CertificatePEM, &keyEncrypted, &cert.SerialNumber,
		&cert.ExpiresAt, &cert.RevokedAt, &cert.NodeID, &cert.CreatedAt)
	if err != nil {
		return MTLSCertificate{}, errors.New("no active CA certificate found")
	}

	cert.CertType = MTLSCertType(certTypeStr)

	if keyEncrypted != "" && s.secrets != nil {
		decrypted, decErr := s.decryptSecret(keyEncrypted, "", secretAAD("mtls_certificates", cert.ID, "private_key"))
		if decErr == nil {
			cert.PrivateKey = decrypted
		}
	}

	return cert, nil
}

func (s *Store) DeleteMTLSCertificate(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM mtls_certificates WHERE id::text = $1`, id)
	return err
}

func (s *Store) GetMTLSStatus(ctx context.Context) (map[string]any, error) {
	var caCount, serverCertCount, clientCertCount, revokedCount, activeCount int
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM mtls_certificates WHERE cert_type = 'ca'`).Scan(&caCount)
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM mtls_certificates WHERE cert_type = 'server'`).Scan(&serverCertCount)
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM mtls_certificates WHERE cert_type = 'client'`).Scan(&clientCertCount)
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM mtls_certificates WHERE revoked_at IS NOT NULL`).Scan(&revokedCount)
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM mtls_certificates WHERE revoked_at IS NULL AND expires_at > NOW()`).Scan(&activeCount)

	var caExpiresAt *time.Time
	_ = s.db.QueryRow(ctx, `SELECT expires_at FROM mtls_certificates WHERE cert_type = 'ca' AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1`).Scan(&caExpiresAt)

	return map[string]any{
		"caConfigured":      caCount > 0,
		"caExpiresAt":       caExpiresAt,
		"serverCertCount":   serverCertCount,
		"clientCertCount":   clientCertCount,
		"revokedCount":      revokedCount,
		"activeCerts":       activeCount,
		"totalCertificates": caCount + serverCertCount + clientCertCount,
	}, nil
}
