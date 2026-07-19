package auth

import (
	"testing"
)

func TestScopes_Contains(t *testing.T) {
	scopes := Scopes{ScopeServerRead, ScopeServerWrite, ScopeAdmin}

	testCases := []struct {
		scope    Scope
		expected bool
	}{
		{ScopeServerRead, true},
		{ScopeServerWrite, true},
		{ScopeAdmin, true},
		{ScopeBackupRead, false},
		{ScopeBackupWrite, false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.scope), func(t *testing.T) {
			result := scopes.Contains(tc.scope)
			if result != tc.expected {
				t.Errorf("Contains(%v) = %v, want %v", tc.scope, result, tc.expected)
			}
		})
	}
}

func TestScopes_Intersect(t *testing.T) {
	scopes1 := Scopes{ScopeServerRead, ScopeServerWrite, ScopeAdmin}
	scopes2 := Scopes{ScopeServerRead, ScopeBackupRead, ScopeAdmin}

	expected := Scopes{ScopeServerRead, ScopeAdmin}
	result := scopes1.Intersect(scopes2)

	if len(result) != len(expected) {
		t.Errorf("Intersect() length = %v, want %v", len(result), len(expected))
	}

	for i, scope := range result {
		if scope != expected[i] {
			t.Errorf("Intersect()[%d] = %v, want %v", i, scope, expected[i])
		}
	}
}

func TestScopes_String(t *testing.T) {
	scopes := Scopes{ScopeServerRead, ScopeServerWrite, ScopeAdmin}
	expected := "server:read, server:write, admin"

	result := scopes.String()
	if result != expected {
		t.Errorf("String() = %v, want %v", result, expected)
	}
}
