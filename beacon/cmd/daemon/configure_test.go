package main

import "testing"

func TestNodeIDRegexAcceptsForgeUUIDAndLegacyNumericID(t *testing.T) {
	for _, value := range []string{"123", "123e4567-e89b-12d3-a456-426614174000"} {
		if !nodeIDRegex.MatchString(value) {
			t.Fatalf("node ID %q should be accepted", value)
		}
	}
	if nodeIDRegex.MatchString("node-1") {
		t.Fatal("non-Forge node ID should be rejected")
	}
}
