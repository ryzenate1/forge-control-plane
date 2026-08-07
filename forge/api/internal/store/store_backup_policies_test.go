package store

import "testing"

// TestValidateBackupPolicyInterval mirrors the
// backup_policies_interval_check constraint: inputs the database would
// refuse must be rejected here so callers can answer 400 instead of 500.
func TestValidateBackupPolicyInterval(t *testing.T) {
	valid := []string{"@daily", "@weekly", "@monthly", "@yearly", "1 day", "30 minutes", "2 hours", "1 week", "3 months", "*/5 * * * *", "0 2 * * *"}
	for _, in := range valid {
		if err := ValidateBackupPolicyInterval(in); err != nil {
			t.Errorf("ValidateBackupPolicyInterval(%q) = %v, want nil", in, err)
		}
	}
	invalid := []string{"", "daily", "every day", "soon", "daily@2am", "@hourly"}
	for _, in := range invalid {
		if err := ValidateBackupPolicyInterval(in); err == nil {
			t.Errorf("ValidateBackupPolicyInterval(%q) = nil, want error", in)
		}
	}
}

func TestValidateBackupPolicyStorage(t *testing.T) {
	for _, in := range []string{"s3", "local", "sftp", "gcs", "azure"} {
		if err := ValidateBackupPolicyStorage(in); err != nil {
			t.Errorf("ValidateBackupPolicyStorage(%q) = %v, want nil", in, err)
		}
	}
	for _, in := range []string{"", "ftp", "disk", "LOCAL"} {
		if err := ValidateBackupPolicyStorage(in); err == nil {
			t.Errorf("ValidateBackupPolicyStorage(%q) = nil, want error", in)
		}
	}
}
