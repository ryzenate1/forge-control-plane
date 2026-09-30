package operations

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const maxInstallerRedirects = 5

var installerExternalRequests = rate.NewLimiter(rate.Limit(10), 20)

// SecureHTTPClient returns an HTTP client that rejects non-HTTP schemes,
// credentials in URLs, redirects to restricted hosts, and connections to
// loopback/private/link-local/multicast addresses. Dialing is pinned to an IP
// that was validated immediately before the connection to prevent DNS rebinding.
func SecureHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("invalid download address: %w", err)
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve download host: %w", err)
			}
			for _, ip := range ips {
				if restrictedDownloadIP(ip) {
					continue
				}
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				err = dialErr
			}
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("download host resolves only to restricted addresses")
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxInstallerRedirects {
				return fmt.Errorf("too many redirects")
			}
			return ValidateDownloadURL(req.URL)
		},
	}
}

func DownloadVerified(ctx context.Context, rawURL, destination, expectedSHA256 string, maxBytes int64, timeout time.Duration) error {
	checksum, err := hex.DecodeString(strings.TrimSpace(expectedSHA256))
	if err != nil || len(checksum) != sha256.Size {
		return errors.New("a valid expected SHA-256 checksum is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if err := ValidateDownloadURL(parsed); err != nil {
		return err
	}
	if maxBytes <= 0 {
		return errors.New("download size limit must be positive")
	}
	client := SecureHTTPClient(timeout)
	resp, err := DoWithRetry(ctx, client, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", resp.Status)
	}
	if resp.ContentLength > maxBytes {
		return fmt.Errorf("download exceeds %d-byte limit", maxBytes)
	}
	requiredSpace := maxBytes
	if resp.ContentLength > 0 {
		requiredSpace = resp.ContentLength
	}
	if err := EnsureDiskSpace(filepath.Dir(destination), requiredSpace); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".download-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temp, hasher), io.LimitReader(resp.Body, maxBytes+1))
	if copyErr != nil || written == 0 || written > maxBytes {
		_ = temp.Close()
		if copyErr != nil {
			return copyErr
		}
		return errors.New("download is empty or exceeds its size limit")
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), expectedSHA256) {
		return errors.New("downloaded file SHA-256 mismatch")
	}
	if err := os.Rename(tempPath, destination); err != nil {
		return err
	}
	return SyncDirectory(filepath.Dir(destination))
}

func EnsureDiskSpace(path string, required int64) error {
	if required <= 0 {
		return errors.New("required disk space must be positive")
	}
	available, err := availableDiskBytes(path)
	if err != nil {
		return fmt.Errorf("check available disk space: %w", err)
	}
	// Retain a small reserve so an installation cannot consume the filesystem
	// to its final byte and prevent logs or rollback metadata from being written.
	const reserve = int64(64 << 20)
	if available < required || available-required < reserve {
		return fmt.Errorf("insufficient disk space: need %d bytes plus %d-byte reserve, have %d", required, reserve, available)
	}
	return nil
}

// DoWithRetry executes an idempotent request up to three times for transient
// transport failures, 429 responses, and 5xx responses.
func DoWithRetry(ctx context.Context, client *http.Client, request func() (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := request()
		if err != nil {
			return nil, err
		}
		if err := installerExternalRequests.Wait(ctx); err != nil {
			return nil, err
		}
		resp, err := client.Do(req.WithContext(ctx))
		if err == nil && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return resp, nil
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("transient HTTP status %s", resp.Status)
		} else {
			lastErr = err
		}
		if attempt == 2 {
			break
		}
		delay := time.Duration(1<<attempt) * 250 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

func ValidateDownloadURL(parsed *url.URL) error {
	if parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return fmt.Errorf("download URL must use http or https")
	}
	if parsed.User != nil {
		return fmt.Errorf("download URL must not contain credentials")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || host == "metadata.google.internal" {
		return fmt.Errorf("download URL targets a restricted host")
	}
	if ip := net.ParseIP(host); ip != nil && restrictedDownloadIP(ip) {
		return fmt.Errorf("download URL targets a restricted address")
	}
	return nil
}

func restrictedDownloadIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return cgnat.Contains(ip)
}
