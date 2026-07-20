package server

import "testing"

func TestValidateApplicationBuildRequestRejectsUnsafeSources(t *testing.T) {
	if err := validateApplicationBuildRequest(testServerID, "https://github.com/example/app.git", "main", "", "Dockerfile", "forge-app:test"); err != nil {
		t.Fatal(err)
	}
	if err := validateApplicationBuildRequest(testServerID, "https://token@github.com/example/app.git", "main", "", "Dockerfile", "forge-app:test"); err == nil {
		t.Fatal("expected embedded credentials to be rejected")
	}
	if err := validateApplicationBuildRequest(testServerID, "https://github.com/example/app.git", "main", "../escape", "Dockerfile", "forge-app:test"); err == nil {
		t.Fatal("expected build path escape to be rejected")
	}
}

func TestAllowedGitHostUsesSafeDefault(t *testing.T) {
	t.Setenv("DAEMON_GIT_ALLOWED_HOSTS", "")
	if !allowedGitHost("github.com") || allowedGitHost("localhost") {
		t.Fatal("unexpected default Git host policy")
	}
}
