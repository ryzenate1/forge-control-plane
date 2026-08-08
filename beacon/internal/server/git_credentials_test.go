package server

import (
	"os"
	"strings"
	"testing"
)

func TestGitEnvironmentUsesRequestScopedAskPassFile(t *testing.T) {
	env, cleanup, err := gitEnvironmentForRequest(gitCloneRequest{
		Username:    "git-user",
		AccessToken: "secret-token",
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var askPassPath string
	for _, value := range env {
		if strings.HasPrefix(value, "GIT_ASKPASS=") {
			askPassPath = strings.TrimPrefix(value, "GIT_ASKPASS=")
			break
		}
	}
	if askPassPath == "" {
		t.Fatal("GIT_ASKPASS was not configured")
	}
	if _, err := os.Stat(askPassPath); err != nil {
		t.Fatalf("askpass file is not available: %v", err)
	}

	cleanup()
	if _, err := os.Stat(askPassPath); !os.IsNotExist(err) {
		t.Fatalf("askpass file was not removed, stat error: %v", err)
	}
}
