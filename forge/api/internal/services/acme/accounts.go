package acme

import (
	"context"
	"encoding/json"

	"gamepanel/forge/internal/store"
)

type DNSAccountService struct {
	store *store.Store
}

func NewDNSAccountService(s *store.Store) *DNSAccountService {
	return &DNSAccountService{store: s}
}

func (s *DNSAccountService) CreateDNSAccount(ctx context.Context, provider, name string, credentials map[string]string) (store.DNSProviderAccount, error) {
	raw, err := json.Marshal(credentials)
	if err != nil {
		return store.DNSProviderAccount{}, err
	}
	return s.store.CreateDNSProviderAccount(ctx, store.CreateDNSProviderAccountRequest{
		Name:        name,
		Provider:    provider,
		Credentials: raw,
	})
}

func (s *DNSAccountService) ListDNSAccounts(ctx context.Context, provider string) ([]store.DNSProviderAccount, error) {
	return s.store.ListDNSProviderAccounts(ctx, provider)
}

func (s *DNSAccountService) GetDNSAccount(ctx context.Context, id string) (store.DNSProviderAccount, error) {
	return s.store.GetDNSProviderAccount(ctx, id)
}

func (s *DNSAccountService) UpdateDNSAccount(ctx context.Context, id string, req store.UpdateDNSProviderAccountRequest) (store.DNSProviderAccount, error) {
	return s.store.UpdateDNSProviderAccount(ctx, id, req)
}

func (s *DNSAccountService) DeleteDNSAccount(ctx context.Context, id string) error {
	return s.store.DeleteDNSProviderAccount(ctx, id)
}
