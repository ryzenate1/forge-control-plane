package vaultprovider

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"

	"gamepanel/forge/internal/store"
)

// Service is the layered seam between the HTTP handlers and the store/client:
// it validates inputs, maps the persisted connection to the client's live view,
// and performs fail-closed secret resolution. Credentials never leave the
// service boundary except inside store.VaultConnection's json:"-" fields, which
// are consumed only to talk to Vault.
type Service struct {
	store  *store.Store
	client *Client
	logger *log.Logger
}

func New(st *store.Store) (*Service, error) {
	if st == nil {
		return nil, errors.New("store required")
	}
	return &Service{store: st, client: NewClient()}, nil
}

func (s *Service) SetLogger(logger *log.Logger) { s.logger = logger }

func (s *Service) log(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf("[vault] "+format, args...)
	}
}

// ---- CRUD (delegating to the store, with input validation on top) ----

func (s *Service) ListConnections(ctx context.Context, orgID *string) ([]store.VaultConnection, error) {
	return s.store.ListVaultConnections(ctx, orgID)
}

func (s *Service) CreateConnection(ctx context.Context, input ConnectionInput) (store.VaultConnection, error) {
	if err := validateConnectionBaseURL(input.BaseURL); err != nil {
		return store.VaultConnection{}, err
	}
	if _, err := ParseAuthMethod(input.AuthMethod); err != nil {
		return store.VaultConnection{}, err
	}
	return s.store.CreateVaultConnection(ctx, store.CreateVaultConnectionRequest{
		OrgID:         input.OrgID,
		Name:          input.Name,
		BaseURL:       input.BaseURL,
		MountPath:     input.MountPath,
		Namespace:     input.Namespace,
		EngineVersion: input.EngineVersion,
		AuthMethod:    input.AuthMethod,
		Token:         derefString(input.Token),
		RoleID:        derefString(input.RoleID),
		SecretID:      derefString(input.SecretID),
		Enabled:       input.Enabled,
	})
}

func (s *Service) UpdateConnection(ctx context.Context, id string, req store.UpdateVaultConnectionRequest) (store.VaultConnection, error) {
	if req.BaseURL != nil {
		if err := validateConnectionBaseURL(*req.BaseURL); err != nil {
			return store.VaultConnection{}, err
		}
	}
	if req.AuthMethod != nil {
		if _, err := ParseAuthMethod(*req.AuthMethod); err != nil {
			return store.VaultConnection{}, err
		}
	}
	return s.store.UpdateVaultConnection(ctx, id, req)
}

func (s *Service) DeleteConnection(ctx context.Context, id string) error {
	return s.store.DeleteVaultConnection(ctx, id)
}

// TestConnection performs a real reachability + auth check against the stored
// connection. It surfaces a genuine error rather than a synthetic success.
func (s *Service) TestConnection(ctx context.Context, id string) error {
	conn, err := s.store.GetVaultConnection(ctx, id)
	if err != nil {
		return fmt.Errorf("vault connection not found: %w", err)
	}
	if err := s.client.TestConnection(ctx, toLiveConnection(conn)); err != nil {
		return err
	}
	return nil
}

// ---- Resolution ----

// Resolve fetches the referenced secret live from Vault. Every failure mode —
// malformed reference, missing/disabled connection, unreachable Vault, absent
// path or field — returns an error; there is no empty-string success.
func (s *Service) Resolve(ctx context.Context, ref VaultRef) (string, error) {
	if !ref.Valid() {
		return "", fmt.Errorf("malformed vault reference %q (expected vault:<connection-id>/<path>#<field>)", RenderVaultReference(ref))
	}
	conn, err := s.store.GetVaultConnection(ctx, ref.ConnectionID)
	if err != nil {
		return "", fmt.Errorf("vault connection %q is not available: %w", ref.ConnectionID, err)
	}
	if !conn.Enabled {
		return "", fmt.Errorf("vault connection %q is disabled", ref.ConnectionID)
	}
	value, err := s.client.ReadField(ctx, toLiveConnection(conn), ref.Path, ref.Field)
	if err != nil {
		return "", fmt.Errorf("resolve vault reference %q: %w", RenderVaultReference(ref), err)
	}
	return value, nil
}

// ResolveIfReference is the seam installed into the environment-variable
// resolver: for a non-reference value it reports (value-ignored, isRef=false,
// nil) so the caller passes the value through unchanged; for a reference it
// returns the live secret (isRef=true) or the resolve error (isRef=true, err).
func (s *Service) ResolveIfReference(ctx context.Context, value string) (string, bool, error) {
	ref, isRef := ParseVaultReference(value)
	if !isRef {
		return "", false, nil
	}
	resolved, err := s.Resolve(ctx, ref)
	if err != nil {
		return "", true, err
	}
	return resolved, true, nil
}

// ---- helpers ----

func toLiveConnection(c store.VaultConnection) VaultConnection {
	auth, _ := ParseAuthMethod(c.AuthMethod)
	namespace := ""
	if c.Namespace != nil {
		namespace = *c.Namespace
	}
	return VaultConnection{
		ID:            c.ID,
		Name:          c.Name,
		BaseURL:       c.BaseURL,
		MountPath:     c.MountPath,
		Namespace:     namespace,
		EngineVersion: c.EngineVersion,
		Auth:          auth,
		Token:         c.Token,
		RoleID:        c.RoleID,
		SecretID:      c.SecretID,
		Enabled:       c.Enabled,
	}
}

func validateConnectionBaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("base URL is required")
	}
	if err := validateBaseURL(raw); err != nil {
		return err
	}
	// Reject paths/queries that would corrupt the /v1 request construction.
	if parsed, err := url.Parse(raw); err == nil {
		if strings.Trim(parsed.Path, "/") != "" {
			return errors.New("base URL must not include a path; set the mount path separately")
		}
	}
	return nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
