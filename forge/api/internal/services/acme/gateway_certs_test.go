package acme

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: the gateway certificate-delivery contract (GatewayCertConfig,
// Service.SetGateway, deliverCertificateToGateway) was removed from this
// package by the refactor — issued/renewed certs are persisted via the store
// and the gateway/traffic layer picks them up from there. Likewise the ACME
// account-email guards (isValidEmail, accounts store, getAccountKeyForRenew)
// are gone: IssueCertificate now simply defaults an empty Email to
// "admin@localhost" instead of rejecting it (so the old email-rejection
// subtests were dropped rather than inverted — asserting the pass-through
// path would reach live ACME registration over the network), and renewOnce
// performs a live network registration so it is no longer unit-testable.
// These tests pin the validation contract that survives: everything
// IssueCertificate rejects *before* any ACME network activity.

func TestIssueCertificateInputValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("requires at least one domain", func(t *testing.T) {
		svc := New(nil, nil)
		_, err := svc.IssueCertificate(ctx, IssueCertificateRequest{Domains: nil, Email: "real@example.com"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one domain is required")
	})

	t.Run("rejects unsupported challenge type", func(t *testing.T) {
		svc := New(nil, nil)
		_, err := svc.IssueCertificate(ctx, IssueCertificateRequest{
			Domains:       []string{"example.com"},
			Email:         "real@example.com",
			ChallengeType: "tls-alpn-01",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported challenge type")
	})

	t.Run("wildcard requires dns-01", func(t *testing.T) {
		svc := New(nil, nil)
		_, err := svc.IssueCertificate(ctx, IssueCertificateRequest{
			Domains:       []string{"*.example.com"},
			Email:         "real@example.com",
			ChallengeType: ChallengeTypeHTTP01,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wildcard certificates require dns-01 challenge")
	})

	t.Run("dns-01 requires provider", func(t *testing.T) {
		svc := New(nil, nil)
		_, err := svc.IssueCertificate(ctx, IssueCertificateRequest{
			Domains:       []string{"example.com"},
			Email:         "real@example.com",
			ChallengeType: ChallengeTypeDNS01,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dns provider is required")
	})

	t.Run("dns-01 requires credentials", func(t *testing.T) {
		svc := New(nil, nil)
		_, err := svc.IssueCertificate(ctx, IssueCertificateRequest{
			Domains:        []string{"example.com"},
			Email:          "real@example.com",
			ChallengeType:  ChallengeTypeDNS01,
			DNSProvider:    "cloudflare",
			DNSCredentials: nil,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dns credentials are required")
	})
}
