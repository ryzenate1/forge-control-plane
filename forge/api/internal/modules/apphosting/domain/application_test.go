package domain

import "testing"

func TestApplicationValidation(t *testing.T) {
	if err := (Application{EnvironmentID: "env", NodeID: "node", Name: "api", Source: SourceImage, Image: "ghcr.io/example/api:1", Deployment: StrategyRolling}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Application{EnvironmentID: "env", NodeID: "node", Name: "api", Source: SourceGit}).Validate(); err == nil {
		t.Fatal("expected missing repository error")
	}
}
