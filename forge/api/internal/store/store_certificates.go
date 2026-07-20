package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Certificate struct {
	ID            string    `json:"id"`
	Domains       []string  `json:"domains"`
	Issuer        string    `json:"issuer"`
	Certificate   string    `json:"certificate"`
	PrivateKey    string    `json:"-"`
	ExpiresAt     time.Time `json:"expiresAt"`
	AutoRenew     bool      `json:"autoRenew"`
	Provider      string    `json:"provider"`
	ChallengeType string    `json:"challengeType"`
	Wildcard      bool      `json:"wildcard"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type CertificateFilter struct {
	Provider *string
	Status   *string
	Wildcard *bool
	Limit    int
	Offset   int
}

type CreateCertificateRequest struct {
	Domains       []string
	Issuer        string
	Certificate   string
	PrivateKey    string
	ExpiresAt     time.Time
	AutoRenew     bool
	Provider      string
	ChallengeType string
	Wildcard      bool
}

type UpdateCertificateRequest struct {
	Certificate *string
	PrivateKey  *string
	ExpiresAt   *time.Time
	AutoRenew   *bool
	UpdatedAt   *time.Time
}

type CertificateAttempt struct {
	ID            string     `json:"id"`
	CertificateID string     `json:"certificateId"`
	AttemptType   string     `json:"attemptType"`
	Status        string     `json:"status"`
	Domains       []string   `json:"domains"`
	ErrorMessage  string     `json:"errorMessage"`
	StartedAt     time.Time  `json:"startedAt"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
	RetryCount    int        `json:"retryCount"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type CreateCertificateAttemptRequest struct {
	CertificateID string
	AttemptType   string
	Domains       []string
}

func (s *Store) CreateCertificate(ctx context.Context, req CreateCertificateRequest) (Certificate, error) {
	if len(req.Domains) == 0 {
		return Certificate{}, errors.New("at least one domain is required")
	}
	id := uuid.NewString()
	now := time.Now().UTC()

	encryptedKey, err := s.encryptSecret(req.PrivateKey, secretAAD("certificates", id, "private_key"))
	if err != nil {
		return Certificate{}, err
	}
	if req.ChallengeType == "" {
		req.ChallengeType = "http-01"
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO certificates (id, domains, issuer, certificate, private_key_encrypted, expires_at, auto_renew, provider, challenge_type, wildcard, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, id, req.Domains, req.Issuer, req.Certificate, encryptedKey, req.ExpiresAt, req.AutoRenew, req.Provider, req.ChallengeType, req.Wildcard, now, now)
	if err != nil {
		return Certificate{}, err
	}
	return Certificate{
		ID: id, Domains: req.Domains, Issuer: req.Issuer, Certificate: req.Certificate,
		ExpiresAt: req.ExpiresAt, AutoRenew: req.AutoRenew, Provider: req.Provider,
		ChallengeType: req.ChallengeType, Wildcard: req.Wildcard, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (s *Store) GetCertificate(ctx context.Context, id string) (Certificate, error) {
	var cert Certificate
	var keyEncrypted string

	err := s.db.QueryRow(ctx, `
		SELECT id::text, domains, COALESCE(issuer,''), COALESCE(certificate,''), COALESCE(private_key_encrypted,''),
		       expires_at, auto_renew, COALESCE(provider,''), COALESCE(challenge_type,'http-01'), COALESCE(wildcard,false),
		       created_at, updated_at
		FROM certificates WHERE id::text = $1
	`, id).Scan(&cert.ID, &cert.Domains, &cert.Issuer, &cert.Certificate, &keyEncrypted,
		&cert.ExpiresAt, &cert.AutoRenew, &cert.Provider, &cert.ChallengeType, &cert.Wildcard,
		&cert.CreatedAt, &cert.UpdatedAt)
	if err != nil {
		return Certificate{}, errors.New("certificate not found")
	}

	if keyEncrypted != "" && s.secrets != nil {
		decrypted, decErr := s.decryptSecret(keyEncrypted, "", secretAAD("certificates", cert.ID, "private_key"))
		if decErr == nil {
			cert.PrivateKey = decrypted
		}
	}

	return cert, nil
}

func (s *Store) ListCertificates(ctx context.Context, filter CertificateFilter) ([]Certificate, error) {
	query := `
		SELECT id::text, domains, COALESCE(issuer,''), COALESCE(certificate,''), '', expires_at,
		       auto_renew, COALESCE(provider,''), COALESCE(challenge_type,'http-01'), COALESCE(wildcard,false),
		       created_at, updated_at
		FROM certificates WHERE 1=1`
	args := []any{}
	argIdx := 1

	if filter.Provider != nil {
		query += " AND provider = $" + itoa(argIdx)
		args = append(args, *filter.Provider)
		argIdx++
	}
	if filter.Wildcard != nil {
		query += " AND wildcard = $" + itoa(argIdx)
		args = append(args, *filter.Wildcard)
		argIdx++
	}
	if filter.Status != nil && *filter.Status == "expiring" {
		query += " AND expires_at <= NOW() + INTERVAL '30 days' AND auto_renew = true"
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

	var certs []Certificate
	for rows.Next() {
		var cert Certificate
		var _pk string
		if err := rows.Scan(&cert.ID, &cert.Domains, &cert.Issuer, &cert.Certificate, &_pk,
			&cert.ExpiresAt, &cert.AutoRenew, &cert.Provider, &cert.ChallengeType, &cert.Wildcard,
			&cert.CreatedAt, &cert.UpdatedAt); err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	return certs, rows.Err()
}

func (s *Store) UpdateCertificate(ctx context.Context, id string, req UpdateCertificateRequest) (Certificate, error) {
	updates := []string{}
	args := []any{}

	if req.Certificate != nil {
		updates = append(updates, "certificate = $"+itoa(len(args)+1))
		args = append(args, *req.Certificate)
	}
	if req.ExpiresAt != nil {
		updates = append(updates, "expires_at = $"+itoa(len(args)+1))
		args = append(args, *req.ExpiresAt)
	}
	if req.AutoRenew != nil {
		updates = append(updates, "auto_renew = $"+itoa(len(args)+1))
		args = append(args, *req.AutoRenew)
	}
	if req.UpdatedAt != nil {
		updates = append(updates, "updated_at = $"+itoa(len(args)+1))
		args = append(args, *req.UpdatedAt)
	}
	if req.PrivateKey != nil && s.secrets != nil {
		encryptedKey, err := s.encryptSecret(*req.PrivateKey, secretAAD("certificates", id, "private_key"))
		if err != nil {
			return Certificate{}, err
		}
		updates = append(updates, "private_key_encrypted = $"+itoa(len(args)+1))
		args = append(args, encryptedKey)
	}

	updates = append(updates, "updated_at = now()")
	args = append(args, id)

	if len(updates) == 0 {
		return s.GetCertificate(ctx, id)
	}

	q := "UPDATE certificates SET " + updates[0]
	for i := 1; i < len(updates); i++ {
		q += ", " + updates[i]
	}
	q += " WHERE id::text = $" + itoa(len(args))

	if _, err := s.db.Exec(ctx, q, args...); err != nil {
		return Certificate{}, err
	}
	return s.GetCertificate(ctx, id)
}

func (s *Store) DeleteCertificate(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM certificates WHERE id::text = $1`, id)
	return err
}

func (s *Store) FindExpiringCertificates(ctx context.Context) ([]Certificate, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, domains, COALESCE(issuer,''), COALESCE(certificate,''), COALESCE(private_key_encrypted,''),
		       expires_at, auto_renew, COALESCE(provider,''), COALESCE(challenge_type,'http-01'), COALESCE(wildcard,false),
		       created_at, updated_at
		FROM certificates
		WHERE auto_renew = true AND expires_at <= NOW() + INTERVAL '30 days'
		ORDER BY expires_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []Certificate
	for rows.Next() {
		var cert Certificate
		var keyEncrypted string
		if err := rows.Scan(&cert.ID, &cert.Domains, &cert.Issuer, &cert.Certificate, &keyEncrypted,
			&cert.ExpiresAt, &cert.AutoRenew, &cert.Provider, &cert.ChallengeType, &cert.Wildcard,
			&cert.CreatedAt, &cert.UpdatedAt); err != nil {
			return nil, err
		}
		if keyEncrypted != "" && s.secrets != nil {
			decrypted, decErr := s.decryptSecret(keyEncrypted, "", secretAAD("certificates", cert.ID, "private_key"))
			if decErr == nil {
				cert.PrivateKey = decrypted
			}
		}
		certs = append(certs, cert)
	}
	return certs, rows.Err()
}

func (s *Store) CreateCertificateAttempt(ctx context.Context, req CreateCertificateAttemptRequest) (CertificateAttempt, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	_, err := s.db.Exec(ctx, `
		INSERT INTO certificate_attempts (id, certificate_id, attempt_type, status, domains, error_message, started_at, completed_at, retry_count, created_at)
		VALUES ($1, $2, $3, 'pending', $4, '', $5, NULL, 0, $6)
	`, id, req.CertificateID, req.AttemptType, req.Domains, now, now)
	if err != nil {
		return CertificateAttempt{}, err
	}
	return CertificateAttempt{
		ID: id, CertificateID: req.CertificateID, AttemptType: req.AttemptType,
		Status: "pending", Domains: req.Domains, StartedAt: now, CreatedAt: now,
	}, nil
}

func (s *Store) UpdateCertificateAttempt(ctx context.Context, id string, status, errorMessage string) error {
	var completedAt *time.Time
	if status == "completed" || status == "failed" {
		now := time.Now().UTC()
		completedAt = &now
	}
	_, err := s.db.Exec(ctx, `
		UPDATE certificate_attempts
		SET status = $1, error_message = $2, completed_at = $3,
		    retry_count = CASE WHEN $4::text = 'failed' THEN retry_count + 1 ELSE retry_count END
		WHERE id::text = $5
	`, status, errorMessage, completedAt, status, id)
	return err
}

func (s *Store) ListCertificateAttempts(ctx context.Context, certificateID string) ([]CertificateAttempt, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, certificate_id::text, COALESCE(attempt_type,'issue'), COALESCE(status,'pending'),
		       domains, COALESCE(error_message,''), started_at, completed_at, COALESCE(retry_count,0), created_at
		FROM certificate_attempts
		WHERE certificate_id::text = $1
		ORDER BY created_at DESC
	`, certificateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []CertificateAttempt
	for rows.Next() {
		var a CertificateAttempt
		if err := rows.Scan(&a.ID, &a.CertificateID, &a.AttemptType, &a.Status,
			&a.Domains, &a.ErrorMessage, &a.StartedAt, &a.CompletedAt, &a.RetryCount, &a.CreatedAt); err != nil {
			return nil, err
		}
		attempts = append(attempts, a)
	}
	return attempts, rows.Err()
}
