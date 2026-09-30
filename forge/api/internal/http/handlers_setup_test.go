package http

import (
	"reflect"
	"testing"
)

func TestNormalizeSetupSkippedSteps(t *testing.T) {
	t.Run("empty stays empty", func(t *testing.T) {
		got, err := normalizeSetupSkippedSteps(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected empty, got %v", got)
		}
	})

	t.Run("normalizes case and whitespace and dedupes", func(t *testing.T) {
		got, err := normalizeSetupSkippedSteps([]string{" Node ", "NODE", "email"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(got, []string{"node", "email"}) {
			t.Fatalf("unexpected normalization: %v", got)
		}
	})

	t.Run("blank entries are ignored", func(t *testing.T) {
		got, err := normalizeSetupSkippedSteps([]string{"", "  ", "backup"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(got, []string{"backup"}) {
			t.Fatalf("unexpected normalization: %v", got)
		}
	})

	t.Run("unknown step is an error, never a silent ignore", func(t *testing.T) {
		if _, err := normalizeSetupSkippedSteps([]string{"administrator"}); err == nil {
			t.Fatal("expected error for non-skippable step")
		}
		if _, err := normalizeSetupSkippedSteps([]string{"billing"}); err == nil {
			t.Fatal("expected error for unknown step")
		}
	})
}

func TestApplySetupSkipsClearsSkippedFields(t *testing.T) {
	req := SetupRequest{
		OrgName: "Acme", NodeName: "n1", NodeFqdn: "n1.example.com",
		SmtpHost: "smtp.example.com", SmtpPort: "587", SmtpUser: "u", SmtpPass: "p",
		SmtpFrom: "a@b.c", SmtpEncryption: "tls",
		BackupDriver: "s3", S3Bucket: "b", S3Region: "r", S3Endpoint: "https://s3.example.com",
		DomainName: "panel.example.com", TlsEmail: "a@b.c",
		Email: "admin@example.com", Password: "long-enough-password",
	}
	applySetupSkips(&req, map[string]bool{"email": true, "domain": true})
	if req.SmtpHost != "" || req.SmtpUser != "" || req.SmtpPass != "" || req.SmtpFrom != "" {
		t.Fatalf("skipped email fields were not cleared: %+v", req)
	}
	if req.DomainName != "" || req.TlsEmail != "" {
		t.Fatalf("skipped domain fields were not cleared: %+v", req)
	}
	// Non-skipped sections keep their values.
	if req.OrgName != "Acme" || req.NodeName != "n1" || req.BackupDriver != "s3" {
		t.Fatalf("non-skipped fields must be preserved: %+v", req)
	}
	if req.Email == "" || req.Password == "" {
		t.Fatal("admin credentials must never be cleared by a skip")
	}
}
