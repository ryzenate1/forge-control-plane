package git

import (
	"context"
	"strings"
	"testing"

	"gamepanel/forge/internal/store"
)

func TestProviderUserEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		provider   store.GitProviderType
		baseURL    string
		wantSuffix string
		wantHeader string
		wantErr    string
	}{
		{"github default", store.GitProviderGitHub, "", "https://api.github.com/user", "Authorization", ""},
		{"github enterprise", store.GitProviderGitHub, "https://ghe.acme.com", "https://ghe.acme.com/api/v3/user", "Authorization", ""},
		{"gitlab default", store.GitProviderGitLab, "", "https://gitlab.com/api/v4/user", "PRIVATE-TOKEN", ""},
		{"gitlab self-hosted", store.GitProviderGitLab, "https://git.acme.com", "https://git.acme.com/api/v4/user", "PRIVATE-TOKEN", ""},
		{"bitbucket", store.GitProviderBitbucket, "", "https://api.bitbucket.org/2.0/user", "Authorization", ""},
		{"gitea", store.GitProviderGitea, "https://git.acme.com", "https://git.acme.com/api/v1/user", "Authorization", ""},
		{"gitea missing base", store.GitProviderGitea, "", "", "", "base URL is required"},
		{"generic unsupported", store.GitProviderGeneric, "", "", "", "not supported"},
		{"empty unsupported", store.GitProviderType(""), "", "", "", "not supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check, err := providerUserEndpoint(tt.provider, tt.baseURL)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if check.endpoint != tt.wantSuffix {
				t.Errorf("endpoint = %q, want %q", check.endpoint, tt.wantSuffix)
			}
			if _, ok := check.headers[tt.wantHeader]; !ok {
				t.Errorf("missing auth header %q in %v", tt.wantHeader, check.headers)
			}
		})
	}
}

func TestValidateProviderTokenRejectsWithoutNetwork(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()

	if _, err := svc.ValidateProviderToken(ctx, store.GitProviderGitHub, "", ""); err == nil {
		t.Errorf("expected error for empty token")
	}
	if _, err := svc.ValidateProviderToken(ctx, store.GitProviderGeneric, "tok", ""); err == nil {
		t.Errorf("expected error for unsupported provider")
	}
	if _, err := svc.ValidateProviderToken(ctx, store.GitProviderGitea, "tok", ""); err == nil {
		t.Errorf("expected error for gitea without base URL")
	}
}
