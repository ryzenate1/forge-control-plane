package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

const maxAttempts = 8

type deliveryResult struct {
	status  *int
	excerpt string
}

func (s *Service) Start(ctx context.Context) {
	if s != nil && s.store != nil {
		s.wg.Add(1)
		go func() { defer s.wg.Done(); s.loop(ctx) }()
	}
}
func (s *Service) loop(ctx context.Context) {
	workerID := "webhook-" + uuid.NewString()
	const (
		minInterval   = time.Second
		maxInterval   = 30 * time.Second
		idleThreshold = 5
	)
	interval := minInterval
	consecutiveIdle := 0
	for {
		if !s.processOne(ctx, workerID) {
			consecutiveIdle++
			if consecutiveIdle >= idleThreshold {
				interval = interval * 2
				if interval > maxInterval {
					interval = maxInterval
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		} else {
			consecutiveIdle = 0
			interval = minInterval
		}
		if ctx.Err() != nil {
			return
		}
	}
}
func (s *Service) processOne(ctx context.Context, workerID string) bool {
	claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	d, err := s.store.ClaimWebhookDelivery(claimCtx, workerID, time.Minute)
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("webhook worker claim: %v", err)
		}
		return false
	}
	if d == nil {
		return false
	}
	sendCtx, sendCancel := context.WithTimeout(ctx, 15*time.Second)
	result, err := sendDelivery(sendCtx, *d, netResolver{net.DefaultResolver})
	sendCancel()
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	if err == nil {
		if e := s.store.CompleteWebhookDelivery(finishCtx, d.ID, workerID, *result.status, result.excerpt); e != nil {
			log.Printf("webhook complete %s: %v", d.ID, e)
		}
		return true
	}
	retry := d.Attempts < maxAttempts
	if e := s.store.FailWebhookDelivery(finishCtx, d.ID, workerID, result.status, result.excerpt, err.Error(), retry, webhookRetryDelay(d.Attempts)); e != nil {
		log.Printf("webhook retry %s: %v", d.ID, e)
	}
	return true
}

func sendDelivery(ctx context.Context, d store.WebhookDelivery, resolver Resolver) (deliveryResult, error) {
	if err := validateURL(ctx, d.TargetURL, resolver); err != nil {
		return deliveryResult{}, err
	}
	client := secureHTTPClient(resolver)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.TargetURL, bytes.NewReader(d.RequestBody))
	if err != nil {
		return deliveryResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if d.WebhookType == store.WebhookTypeRegular {
		req.Header.Set("X-Webhook-Event", d.EventName)
		if d.WebhookID != nil {
			req.Header.Set("X-Webhook-Id", *d.WebhookID)
		}
		if d.Secret != "" {
			req.Header.Set("X-Webhook-Signature", webhookSignature(d.Secret, d.Payload))
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return deliveryResult{}, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4097))
	excerpt := string(body)
	if len(excerpt) > 4000 {
		excerpt = excerpt[:4000]
	}
	status := resp.StatusCode
	result := deliveryResult{status: &status, excerpt: excerpt}
	if readErr != nil {
		return result, fmt.Errorf("read webhook response: %w", readErr)
	}
	if status < 200 || status >= 300 {
		return result, fmt.Errorf("webhook target responded %d", status)
	}
	return result, nil
}

func secureHTTPClient(resolver Resolver) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: -1}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
			if forbiddenIP(ip) {
				return nil, errorsPublicAddress()
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}
		ips, err := resolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("host has no addresses")
		}
		for _, ip := range ips {
			if forbiddenIP(ip) {
				return nil, errorsPublicAddress()
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return validateURL(req.Context(), req.URL.String(), resolver)
	}}
}
func webhookSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func errorsPublicAddress() error { return fmt.Errorf("webhook target is not a public address") }
func webhookRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	return time.Duration(1<<uint(attempt-1)) * time.Minute
}
