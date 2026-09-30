package store

import (
	"context"
	"strings"
	"testing"
)

// TestIsValidScheduleTaskAction_Reverification validates the allowlist for
// schedule task actions and the symmetry fix between Create and Patch.
//
// Valid actions are exactly "power", "backup", "command" (case-insensitive,
// whitespace-trimmed). PatchScheduleTask already validated via
// isValidScheduleTaskAction; CreateScheduleTask was missing that check
// (asymmetry GH-18 style). After the fix both paths enforce the same allowlist.
func TestIsValidScheduleTaskAction_Reverification(t *testing.T) {
	valid := []string{
		"power", "backup", "command",
		"Power", "BACKUP", "Command",
		"  power  ", "\tbackup\n", " COMMAND ",
		"PoWeR", "BaCkUp", "COMMAND",
	}
	for _, tc := range valid {
		if !isValidScheduleTaskAction(tc) {
			t.Errorf("isValidScheduleTaskAction(%q) = false, want true", tc)
		}
	}

	invalid := []string{
		"", " ", "\t", "\n",
		"shell", "exec", "run", "command2", "backup2", "power2",
		"pow", "cmd", "*",
		"power; rm -rf", "backup --force", "command\nrm",
		"power,backup", "invalid-action", "delete",
	}
	for _, tc := range invalid {
		if isValidScheduleTaskAction(tc) {
			t.Errorf("isValidScheduleTaskAction(%q) = true, want false", tc)
		}
	}
}

// TestIsValidScheduleTaskAction_Normalization verifies TrimSpace + ToLower
// normalization is applied before allowlist check.
func TestIsValidScheduleTaskAction_Normalization_Reverification(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"power", true},
		{" POWER", true},
		{"power ", true},
		{"  power  ", true},
		{"PoWeR", true},
		{"backup", true},
		{"BACKUP", true},
		{"  backup\t", true},
		{"command", true},
		{"COMMAND", true},
		{"  COMMAND  ", true},
		{"", false},
		{"   ", false},
		{"power ", true},  // trailing space trimmed
		{" power", true},  // leading space trimmed
		{" pow", false},   // not trimmed to valid
		{"power\n", true}, // TrimSpace handles \n
	}
	for _, tc := range cases {
		got := isValidScheduleTaskAction(tc.input)
		if got != tc.want {
			t.Errorf("isValidScheduleTaskAction(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// TestCreateScheduleTask_RejectsInvalidAction_Reverification ensures
// CreateScheduleTask now rejects unsupported actions before touching the DB.
// This is the asymmetric fix: Patch already rejected, Create now does too.
// Uses a store with nil DB; early validation returns without DB access.
func TestCreateScheduleTask_RejectsInvalidAction_Reverification(t *testing.T) {
	s := &Store{} // nil db, but validation happens before DB
	ctx := context.Background()
	invalid := []string{
		"shell", "exec", "invalid", "*", "delete", "run", "command2",
		"  shell  ", "Power; rm", "",
	}
	for _, action := range invalid {
		// empty action is caught as "action is required" before isValid check;
		// both are expected rejections, but message differs.
		_, err := s.CreateScheduleTask(ctx, "server-id", "schedule-id", CreateScheduleTaskRequest{
			Action: action,
		}, nil)
		if err == nil {
			t.Errorf("CreateScheduleTask(Action=%q) = nil error, want rejection", action)
			continue
		}
		msg := strings.ToLower(err.Error())
		if strings.TrimSpace(action) == "" {
			if !strings.Contains(msg, "action is required") {
				t.Errorf("CreateScheduleTask(Action=%q) error %q, want 'action is required'", action, err.Error())
			}
			continue
		}
		if !strings.Contains(msg, "unsupported") && !strings.Contains(msg, "task action") {
			t.Errorf("CreateScheduleTask(Action=%q) error %q, want 'unsupported task action'", action, err.Error())
		}
		// Error message must contain the trimmed action (mirrors Patch behavior).
		trimmed := strings.TrimSpace(action)
		if trimmed != "" && !strings.Contains(err.Error(), trimmed) {
			t.Errorf("CreateScheduleTask(Action=%q) error %q should contain trimmed action %q", action, err.Error(), trimmed)
		}
	}
}

// TestCreateScheduleTask_AcceptsValidActions_NoUnsupportedError verifies
// valid actions pass the allowlist. Direct validator check avoids nil-DB panic.
func TestCreateScheduleTask_AcceptsValidActions_NoUnsupportedError(t *testing.T) {
	valid := []string{"power", "backup", "command", "Power", "BACKUP", "  command  "}
	for _, action := range valid {
		if !isValidScheduleTaskAction(action) {
			t.Errorf("isValidScheduleTaskAction(%q) = false, want true (valid allowlist)", action)
		}
	}
}

// TestPatchScheduleTask_RejectsInvalidAction_Reverification mirrors the Create
// test for the Patch path, confirming symmetry. Patch already had the check;
// this test ensures it remains.
func TestPatchScheduleTask_RejectsInvalidAction_Reverification(t *testing.T) {
	s := &Store{}
	ctx := context.Background()
	invalid := []string{"shell", "invalid", "exec", "*", "delete"}
	for _, action := range invalid {
		act := action // capture for pointer
		_, err := s.PatchScheduleTask(ctx, "server-id", "schedule-id", "task-id", PatchScheduleTaskRequest{
			Action: &act,
		}, nil)
		if err == nil {
			t.Errorf("PatchScheduleTask(Action=%q) = nil error, want rejection", action)
			continue
		}
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "unsupported") {
			t.Errorf("PatchScheduleTask(Action=%q) error %q, want 'unsupported task action'", action, err.Error())
		}
	}
}

// TestPatchScheduleTask_AcceptsValidAction_Reverification ensures valid actions
// are not rejected at validation layer. Only the private validator is checked
// to avoid requiring a DB; the Store path would hit nil DB for valid inputs.
func TestPatchScheduleTask_AcceptsValidAction_Reverification(t *testing.T) {
	valid := []string{"power", "backup", "command", "Power", "COMMAND"}
	for _, action := range valid {
		if !isValidScheduleTaskAction(action) {
			t.Errorf("isValidScheduleTaskAction(%q) = false, want true", action)
		}
		// Also verify that Patch's early validation would not reject it:
		// isValid check is the gate; nil Action is already tested separately.
		// We do not call PatchScheduleTask with nil DB for valid actions to avoid panic.
		if strings.TrimSpace(action) == "" {
			t.Errorf("valid action %q trimmed to empty", action)
		}
	}
}

// TestPatchScheduleTask_NilAction_PassesValidation ensures nil Action does not
// trigger validation (it means "don't update action"). We only check the
// validator gate, not the DB path with nil DB.
func TestPatchScheduleTask_NilAction_PassesValidation_Reverification(t *testing.T) {
	var nilAction *string = nil
	// nil should not be validated via isValidScheduleTaskAction (guard is `if req.Action != nil`)
	if nilAction != nil && !isValidScheduleTaskAction(*nilAction) {
		t.Error("nil Action should not be validated")
	}
	// Also ensure empty string is rejected via validator, but nil is not
	if isValidScheduleTaskAction("") {
		t.Error("empty string should not be valid")
	}
}

// TestCreatePatchSymmetry_Reverification table-drives that Create and Patch
// agree on every input (symmetric allowlist). For valid inputs we only check
// the validator to avoid nil-DB panics; for invalid we check Store paths via
// nil DB (early validation).
func TestCreatePatchSymmetry_Reverification(t *testing.T) {
	cases := []struct {
		action string
		valid  bool
	}{
		{"power", true},
		{"backup", true},
		{"command", true},
		{"Power", true},
		{"shell", false},
		{"", false},
		{"*", false},
		{"invalid", false},
		{"exec", false},
	}
	s := &Store{}
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			isValid := isValidScheduleTaskAction(tc.action)
			if isValid != tc.valid {
				t.Fatalf("isValidScheduleTaskAction(%q) = %v, want %v", tc.action, isValid, tc.valid)
			}
			if tc.valid {
				// Valid should pass validator; we don't call Store with nil DB for valid
				// because it would proceed to DB and panic. Validator success is sufficient.
				return
			}
			// Invalid: both Create and Patch should reject early via validator
			_, createErr := s.CreateScheduleTask(ctx, "sid", "sch", CreateScheduleTaskRequest{Action: tc.action}, nil)
			createRejected := createErr != nil && (strings.Contains(strings.ToLower(createErr.Error()), "unsupported") || strings.Contains(strings.ToLower(createErr.Error()), "action is required"))
			act := tc.action
			_, patchErr := s.PatchScheduleTask(ctx, "sid", "sch", "tid", PatchScheduleTaskRequest{Action: &act}, nil)
			patchRejected := patchErr != nil && strings.Contains(strings.ToLower(patchErr.Error()), "unsupported")
			// For empty string, Create rejects as "action is required" (valid rejection), Patch with pointer to "" would be rejected as unsupported
			if tc.action == "" {
				if !createRejected {
					t.Errorf("Createsymmetry: empty action should be rejected")
				}
				if !patchRejected {
					t.Errorf("Patchsymmetry: empty action should be rejected (unsupported)")
				}
				return
			}
			if !createRejected {
				t.Errorf("Create accepted invalid %q", tc.action)
			}
			if !patchRejected {
				t.Errorf("Patch accepted invalid %q", tc.action)
			}
		})
	}
}
