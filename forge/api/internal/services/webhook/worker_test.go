package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestWebhookSignatureAndRetryBackoff(t *testing.T) {
	payload := []byte(`{"event":"server:started"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(payload)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got := webhookSignature("secret", payload); got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
	if webhookRetryDelay(2) <= webhookRetryDelay(1) || webhookRetryDelay(1) != time.Minute {
		t.Fatal("webhook retry delay must begin at one minute and increase exponentially")
	}
}
