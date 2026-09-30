package trafficmanager

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func caddyHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func caddyAdminBaseURL(addr string) (*url.URL, bool, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = "localhost:2019"
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	base, err := url.Parse(addr)
	if err != nil || base.Host == "" || base.User != nil {
		return nil, false, errors.New("invalid Caddy admin URL")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, false, errors.New("Caddy admin URL must use HTTP or HTTPS")
	}
	host := base.Hostname()
	ip := net.ParseIP(host)
	isLoopback := strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
	if base.Scheme == "http" && !isLoopback {
		return nil, false, errors.New("remote Caddy admin URL must use HTTPS")
	}
	base.RawQuery = ""
	base.Fragment = ""
	base.Path = strings.TrimRight(base.Path, "/")
	return base, isLoopback, nil
}

func newCaddyAdminRequest(ctx context.Context, method, addr, path string, body io.Reader) (*http.Request, error) {
	base, isLoopback, err := caddyAdminBaseURL(addr)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(os.Getenv("CADDY_ADMIN_TOKEN"))
	if !isLoopback && token == "" {
		return nil, errors.New("CADDY_ADMIN_TOKEN is required for remote Caddy administration")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, method, base.String(), body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}
