package store

import (
	"reflect"
	"testing"
)

func TestAPIKeyIPAllowed(t *testing.T) {
	tests := []struct {
		name       string
		configured []string
		requestIP  string
		want       bool
		wantErr    bool
	}{
		{name: "unrestricted", requestIP: "203.0.113.10", want: true},
		{name: "exact IPv4", configured: []string{"203.0.113.10"}, requestIP: "203.0.113.10", want: true},
		{name: "exact IPv6 normalized", configured: []string{"2001:db8::1"}, requestIP: "2001:0db8:0:0:0:0:0:1", want: true},
		{name: "IPv4 CIDR", configured: []string{"10.20.0.0/16"}, requestIP: "10.20.3.4", want: true},
		{name: "IPv6 CIDR", configured: []string{"2001:db8:abcd::/48"}, requestIP: "2001:db8:abcd::42", want: true},
		{name: "outside CIDR", configured: []string{"10.20.0.0/16"}, requestIP: "10.21.3.4", want: false},
		{name: "malformed request IP", configured: []string{"10.20.0.0/16"}, requestIP: "not-an-ip", wantErr: true},
		{name: "malformed configured entry fails closed before match", configured: []string{"203.0.113.10", "broken"}, requestIP: "203.0.113.10", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := apiKeyIPAllowed(tt.configured, tt.requestIP)
			if (err != nil) != tt.wantErr {
				t.Fatalf("apiKeyIPAllowed() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("apiKeyIPAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeAllowedIPs(t *testing.T) {
	got, err := normalizeAllowedIPs([]string{" 192.0.2.9 ", "10.4.5.6/8", "2001:0db8::1", "192.0.2.9"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.0.0.0/8", "192.0.2.9", "2001:db8::1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeAllowedIPs() = %#v, want %#v", got, want)
	}
	if _, err := normalizeAllowedIPs([]string{""}); err == nil {
		t.Fatal("expected empty restriction to be rejected")
	}
}

func TestValidateAPIKeyScopesByRole(t *testing.T) {
	for _, scope := range []string{"*", "nodes.read", "settings.read", "users.read"} {
		if _, err := ValidateApiKeyScopes([]string{scope}, false); err == nil {
			t.Fatalf("normal user was allowed to issue %q", scope)
		}
	}

	got, err := ValidateApiKeyScopes([]string{"servers.write", "servers.read", "servers.read"}, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"servers.read", "servers.write"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ValidateApiKeyScopes() = %#v, want %#v", got, want)
	}

	for _, scope := range []string{"*", "nodes.read", "settings.read"} {
		if _, err := ValidateApiKeyScopes([]string{scope}, true); err != nil {
			t.Fatalf("admin scope %q was rejected: %v", scope, err)
		}
	}
	if _, err := ValidateApiKeyScopes([]string{"made.up"}, true); err == nil {
		t.Fatal("expected unknown admin scope to be rejected")
	}
}
