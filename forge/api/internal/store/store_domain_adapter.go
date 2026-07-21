package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/services/domains"
)

type DomainNodeResolver struct {
	store *Store
}

func NewDomainNodeResolver(s *Store) *DomainNodeResolver {
	return &DomainNodeResolver{store: s}
}

func (r *DomainNodeResolver) ResolveServerTarget(ctx context.Context, serverID string) (string, int, error) {
	server, err := r.store.GetServer(ctx, serverID)
	if err != nil {
		return "", 0, fmt.Errorf("get server %s: %w", serverID, err)
	}
	node, err := r.store.GetNode(ctx, server.NodeID)
	if err != nil {
		return "", 0, fmt.Errorf("get node %s: %w", server.NodeID, err)
	}
	host := strings.TrimSpace(node.PublicHostname)
	if host == "" {
		host = strings.TrimSpace(node.FQDN)
	}
	if host == "" {
		host = strings.TrimSpace(node.BaseURL)
	}
	if host == "" {
		return "", 0, fmt.Errorf("node %s has no public hostname, fqdn, or base url", server.NodeID)
	}

	// Try to get the port from the server's primary allocation.
	if server.PrimaryAllocationID != nil && *server.PrimaryAllocationID != "" {
		allocations, err := r.store.ListServerAllocations(ctx, serverID)
		if err == nil {
			for _, a := range allocations {
				if a.ID == *server.PrimaryAllocationID {
					return host, a.Port, nil
				}
			}
		}
	}

	return host, 8080, nil
}

type DomainAdapter struct {
	store *Store
}

func NewDomainAdapter(s *Store) *DomainAdapter {
	return &DomainAdapter{store: s}
}

func (a *DomainAdapter) CheckServerExists(ctx context.Context, id string) (bool, error) {
	_, err := a.store.GetServer(ctx, id)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (a *DomainAdapter) ListDomainsByServer(ctx context.Context, serverID string) ([]domains.DomainRow, error) {
	rows, err := a.store.ListDomainsByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}
	result := make([]domains.DomainRow, 0, len(rows))
	for _, r := range rows {
		result = append(result, toDomainRow(r))
	}
	return result, nil
}

func (a *DomainAdapter) ListAllDomains(ctx context.Context) ([]domains.DomainRow, error) {
	rows, err := a.store.ListAllDomains(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domains.DomainRow, 0, len(rows))
	for _, r := range rows {
		result = append(result, toDomainRow(r))
	}
	return result, nil
}

func (a *DomainAdapter) GetDomain(ctx context.Context, id string) (*domains.DomainRow, error) {
	row, err := a.store.GetDomain(ctx, id)
	if err != nil || row == nil {
		return nil, err
	}
	r := toDomainRow(*row)
	return &r, nil
}

func (a *DomainAdapter) GetDomainByDomain(ctx context.Context, domain string) (*domains.DomainRow, error) {
	row, err := a.store.GetDomainByDomain(ctx, domain)
	if err != nil || row == nil {
		return nil, err
	}
	r := toDomainRow(*row)
	return &r, nil
}

func (a *DomainAdapter) CreateDomain(ctx context.Context, row domains.DomainRow) error {
	return a.store.CreateDomain(ctx, fromDomainRow(row))
}

func (a *DomainAdapter) UpdateDomainVerification(ctx context.Context, id string, verified bool, verifiedAt *time.Time) error {
	return a.store.UpdateDomainVerification(ctx, id, verified, verifiedAt)
}

func (a *DomainAdapter) SetDomainVerificationToken(ctx context.Context, id string, token string) error {
	return a.store.SetDomainVerificationToken(ctx, id, token)
}

func (a *DomainAdapter) DeleteDomain(ctx context.Context, id string) error {
	return a.store.DeleteDomain(ctx, id)
}

func (a *DomainAdapter) ListUnverifiedDomains(ctx context.Context) ([]domains.DomainRow, error) {
	rows, err := a.store.ListUnverifiedDomains(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domains.DomainRow, 0, len(rows))
	for _, r := range rows {
		result = append(result, toDomainRow(r))
	}
	return result, nil
}

func (a *DomainAdapter) ListVerifiedDomains(ctx context.Context) ([]domains.DomainRow, error) {
	rows, err := a.store.ListVerifiedDomains(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domains.DomainRow, 0, len(rows))
	for _, r := range rows {
		result = append(result, toDomainRow(r))
	}
	return result, nil
}

func toDomainRow(r DomainRow) domains.DomainRow {
	return domains.DomainRow{
		ID:                r.ID,
		ServerID:          r.ServerID,
		Domain:            r.Domain,
		Wildcard:          r.Wildcard,
		Verified:          r.Verified,
		VerifiedAt:        r.VerifiedAt,
		VerificationToken: r.VerificationToken,
		CreatedAt:         r.CreatedAt,
	}
}

func fromDomainRow(r domains.DomainRow) DomainRow {
	return DomainRow{
		ID:                r.ID,
		ServerID:          r.ServerID,
		Domain:            r.Domain,
		Wildcard:          r.Wildcard,
		Verified:          r.Verified,
		VerifiedAt:        r.VerifiedAt,
		VerificationToken: r.VerificationToken,
		CreatedAt:         r.CreatedAt,
	}
}
