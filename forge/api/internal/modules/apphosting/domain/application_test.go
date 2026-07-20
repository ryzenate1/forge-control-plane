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

func TestGitApplicationRequiresSafeHTTPSSource(t *testing.T) {
	valid := Application{EnvironmentID: "env", NodeID: "node", Name: "api", Source: SourceGit, RepositoryURL: "https://github.com/example/api.git", BaseDirectory: "apps/api", DockerfilePath: "Dockerfile"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.RepositoryURL = "https://token@github.com/example/api.git"
	if err := valid.Validate(); err == nil {
		t.Fatal("expected embedded repository credentials to be rejected")
	}
}
