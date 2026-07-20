package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"gamepanel/forge/internal/services/git"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func testGitService(t *testing.T) *git.Service {
	return nil
}

func TestVerifyGitHubSignature(t *testing.T) {
	payload := []byte(`{"ref":"refs/heads/main","after":"abc123"}`)
	secret := "test-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	err := git.VerifyGitHubSignature(payload, sig, secret)
	if err != nil {
		t.Errorf("expected valid signature, got: %v", err)
	}

	err = git.VerifyGitHubSignature(payload, "", secret)
	if err != git.ErrWebhookSignatureMissing {
		t.Errorf("expected ErrWebhookSignatureMissing, got: %v", err)
	}

	err = git.VerifyGitHubSignature(payload, "sha256=badsig", secret)
	if err != git.ErrWebhookSignatureInvalid {
		t.Errorf("expected ErrWebhookSignatureInvalid, got: %v", err)
	}

	err = git.VerifyGitHubSignature(payload, sig, "wrong-secret")
	if err != git.ErrWebhookSignatureInvalid {
		t.Errorf("expected ErrWebhookSignatureInvalid for wrong secret, got: %v", err)
	}
}

func TestVerifyGitLabSignature(t *testing.T) {
	payload := []byte(`{"object_kind":"push","ref":"refs/heads/main"}`)
	secret := "test-token"

	err := git.VerifyGitLabSignature(payload, secret, secret)
	if err != nil {
		t.Errorf("expected valid token match, got: %v", err)
	}

	err = git.VerifyGitLabSignature(payload, "", secret)
	if err != git.ErrWebhookSignatureMissing {
		t.Errorf("expected ErrWebhookSignatureMissing, got: %v", err)
	}

	err = git.VerifyGitLabSignature(payload, "wrong-token", secret)
	if err != git.ErrWebhookSignatureInvalid {
		t.Errorf("expected ErrWebhookSignatureInvalid, got: %v", err)
	}
}

func TestVerifyGiteaSignature(t *testing.T) {
	payload := []byte(`{"ref":"refs/heads/main"}`)
	secret := "test-secret"

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	err := git.VerifyGiteaSignature(payload, sig, secret)
	if err != nil {
		t.Errorf("expected valid signature, got: %v", err)
	}

	err = git.VerifyGiteaSignature(payload, "", secret)
	if err != git.ErrWebhookSignatureMissing {
		t.Errorf("expected ErrWebhookSignatureMissing, got: %v", err)
	}
}

func TestGitHubWebhookHandler(t *testing.T) {
	app := fiber.New()
	cfg := Config{Store: nil}

	app.Post("/git/webhook/github", HandleGitHubWebhook(cfg))

	t.Run("ping event returns ok", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/git/webhook/github", strings.NewReader(`{"zen":"test"}`))
		req.Header.Set("X-GitHub-Event", "ping")
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("push event without store returns ok", func(t *testing.T) {
		body := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"user/repo","clone_url":"https://github.com/user/repo.git"},"commits":[{"id":"abc123","message":"test","author":{"name":"test","email":"test@test.com"}}]}`
		req := httptest.NewRequest("POST", "/git/webhook/github", strings.NewReader(body))
		req.Header.Set("X-GitHub-Event", "push")
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestGitLabWebhookHandler(t *testing.T) {
	app := fiber.New()
	cfg := Config{Store: nil}

	app.Post("/git/webhook/gitlab", HandleGitLabWebhook(cfg))

	t.Run("push event returns ok", func(t *testing.T) {
		body := `{"object_kind":"push","ref":"refs/heads/main","project":{"git_http_url":"https://gitlab.com/user/repo.git"},"commits":[{"id":"abc123","message":"test","author":{"name":"test","email":"test@test.com"}}]}`
		req := httptest.NewRequest("POST", "/git/webhook/gitlab", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestBitbucketWebhookHandler(t *testing.T) {
	app := fiber.New()
	cfg := Config{Store: nil}

	app.Post("/git/webhook/bitbucket", HandleBitbucketWebhook(cfg))

	t.Run("repo:push event returns ok", func(t *testing.T) {
		body := `{"push":{"changes":[{"new":{"name":"main","target":{"hash":"abc123"}},"commits":[{"hash":"abc123","message":"test","author":{"user":{"display_name":"test"}}}]}]},"repository":{"full_name":"user/repo","links":{"clone":[{"name":"https","href":"https://bitbucket.org/user/repo.git"}]}}}`
		req := httptest.NewRequest("POST", "/git/webhook/bitbucket", strings.NewReader(body))
		req.Header.Set("X-Event-Key", "repo:push")
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestGiteaWebhookHandler(t *testing.T) {
	app := fiber.New()
	cfg := Config{Store: nil}

	app.Post("/git/webhook/gitea", HandleGiteaWebhook(cfg))

	t.Run("push event returns ok", func(t *testing.T) {
		body := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"user/repo","clone_url":"https://gitea.com/user/repo.git"},"commits":[{"id":"abc123","message":"test","author":{"name":"test","email":"test@test.com"}}]}`
		req := httptest.NewRequest("POST", "/git/webhook/gitea", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestDeployKeyGeneration(t *testing.T) {
	svc := git.Service{}
	kp, err := svc.GenerateDeployKeyPair("ed25519", 0)
	if err != nil {
		t.Fatalf("GenerateDeployKeyPair failed: %v", err)
	}
	if kp.PublicKey == "" {
		t.Error("expected public key")
	}
	if kp.PrivateKey == "" {
		t.Error("expected private key")
	}
	if kp.Type != "ed25519" {
		t.Errorf("expected ed25519 type, got %s", kp.Type)
	}
	if !strings.HasPrefix(kp.PublicKey, "ssh-ed25519 ") {
		t.Errorf("expected SSH public key format, got: %s", kp.PublicKey)
	}

	kp2, err := svc.GenerateDeployKeyPair("rsa", 2048)
	if err != nil {
		t.Fatalf("GenerateDeployKeyPair rsa failed: %v", err)
	}
	if kp2.Type != "rsa" {
		t.Errorf("expected rsa type, got %s", kp2.Type)
	}
	if kp2.Bits != 2048 {
		t.Errorf("expected 2048 bits, got %d", kp2.Bits)
	}

	_, err = svc.GenerateDeployKeyPair("invalid", 0)
	if err == nil {
		t.Error("expected error for invalid key type")
	}
}

func TestWebhookHandlerJSONPayloads(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		headers map[string]string
	}{
		{"github ping", "/git/webhook/github", map[string]string{"X-GitHub-Event": "ping", "Content-Type": "application/json"}},
		{"github push", "/git/webhook/github", map[string]string{"X-GitHub-Event": "push", "Content-Type": "application/json"}},
		{"gitlab push", "/git/webhook/gitlab", map[string]string{"Content-Type": "application/json"}},
		{"bitbucket push", "/git/webhook/bitbucket", map[string]string{"X-Event-Key": "repo:push", "Content-Type": "application/json"}},
		{"gitea push", "/git/webhook/gitea", map[string]string{"Content-Type": "application/json"}},
	}

	app := fiber.New()
	cfg := Config{Store: nil}
	registerGitWebhookRoutes(app.Group(""), cfg)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tt.url, strings.NewReader("{}"))
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != 200 {
				t.Errorf("expected 200 for %s, got %d", tt.name, resp.StatusCode)
			}
		})
	}
}

func TestGenerateWebhookSecret(t *testing.T) {
	s1 := generateWebhookSecret()
	s2 := generateWebhookSecret()
	if s1 == s2 {
		t.Error("expected unique secrets")
	}
	if len(s1) != 64 {
		t.Errorf("expected 64-char hex secret, got %d chars", len(s1))
	}
}

func init() {
	_ = json.Valid
	_ = uuid.New
}
