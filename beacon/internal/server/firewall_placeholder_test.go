package server

import "testing"

func TestFirewallPlaceholder_Rejected(t *testing.T) {
	// The previous UI placeholder "0.0.0.0/0" must be rejected as unrestricted (handlers_firewall.go:132).
	if err := validateFirewallSource("0.0.0.0/0"); err == nil {
		t.Fatal("expected 0.0.0.0/0 to be rejected as unrestricted")
	}
	if err := validateFirewallSource("0.0.0.0"); err == nil {
		t.Fatal("expected 0.0.0.0 to be rejected as non-public")
	}
	if err := validateFirewallSource(""); err == nil {
		t.Fatal("expected empty source to be rejected")
	}
}

func TestFirewallPlaceholder_AcceptsValidCIDR(t *testing.T) {
	// Valid public canonical CIDR and single IP must be accepted.
	if err := validateFirewallSource("203.0.113.42"); err != nil {
		t.Fatalf("expected public IP 203.0.113.42 to be accepted: %v", err)
	}
	if err := validateFirewallSource("198.51.100.0/24"); err != nil {
		t.Fatalf("expected public CIDR 198.51.100.0/24 to be accepted: %v", err)
	}
	// Non-canonical must be rejected.
	if err := validateFirewallSource("198.51.100.1/24"); err == nil {
		t.Fatal("expected non-canonical 198.51.100.1/24 to be rejected")
	}
}

func TestFirewallAction_AllowOnly(t *testing.T) {
	// Allow is valid, deny/drop are dead and must be rejected with allow-only error.
	if _, err := validateFirewallAction("allow"); err != nil {
		t.Fatalf("allow should be valid: %v", err)
	}
	if _, err := validateFirewallAction(""); err != nil {
		t.Fatalf("empty should default to allow: %v", err)
	}
	if _, err := validateFirewallAction("deny"); err == nil {
		t.Fatal("expected deny to be rejected (allow-only)")
	}
	if _, err := validateFirewallAction("drop"); err == nil {
		t.Fatal("expected drop to be rejected (allow-only)")
	}
	if _, err := validateFirewallAction("REJECT"); err == nil {
		t.Fatal("expected REJECT to be rejected")
	}
}

func TestFirewallDocs_Placeholder(t *testing.T) {
	// Document that the UI no longer uses 0.0.0.0/0 placeholder; ensure error
	// messages guide operators to valid examples.
	err := validateFirewallSource("0.0.0.0/0")
	if err == nil {
		t.Fatal("placeholder must be rejected")
	}
	msg := err.Error()
	if len(msg) < 10 || !contains(msg, "0.0.0.0/0") {
		t.Fatalf("error message should mention rejected placeholder and suggest valid example, got %q", msg)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
