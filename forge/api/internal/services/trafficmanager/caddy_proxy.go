package trafficmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type CaddyReverseProxy struct {
	adminAddr string
	client    *http.Client
}

func NewCaddyReverseProxy(adminAddr string) *CaddyReverseProxy {
	return &CaddyReverseProxy{
		adminAddr: adminAddr,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *CaddyReverseProxy) UpdateRoutes(ctx context.Context, rules []*RoutingRule) error {
	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	// Ensure the server group exists
	serverConfig := map[string]any{
		"listen": []string{":80", ":443"},
		"routes": []any{},
	}
	initBody, _ := json.Marshal(serverConfig)
	initReq, _ := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://%s/config/apps/http/servers/gamepanel", addr),
		bytes.NewReader(initBody))
	initReq.Header.Set("Content-Type", "application/json")
	if resp, err := p.client.Do(initReq); err == nil {
		resp.Body.Close()
	}

	routes := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		route := map[string]any{
			"@id": "gamepanel-" + rule.ID,
			"match": []map[string]any{
				{
					"host": []string{rule.Domain},
					"path": []string{rule.Path + "*"},
				},
			},
			"handle": []map[string]any{
				{
					"handler":   "reverse_proxy",
					"upstreams": []map[string]any{
						{
							"dial": fmt.Sprintf("localhost:%d", rule.TargetPort),
						},
					},
				},
			},
		}
		routes = append(routes, route)
	}

	body, err := json.Marshal(routes)
	if err != nil {
		return fmt.Errorf("caddy marshal routes: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT",
		fmt.Sprintf("http://%s/config/apps/http/servers/gamepanel/routes", addr),
		bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("caddy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("caddy admin api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caddy admin api error: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

func (p *CaddyReverseProxy) RemoveRoutes(ctx context.Context, ruleIDs []string) error {
	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	for _, id := range ruleIDs {
		req, err := http.NewRequestWithContext(ctx, "DELETE",
			fmt.Sprintf("http://%s/config/apps/http/servers/gamepanel/routes/@id/gamepanel-%s", addr, id),
			nil)
		if err != nil {
			return fmt.Errorf("caddy delete request: %w", err)
		}
		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("caddy delete api: %w", err)
		}
		resp.Body.Close()
	}
	return nil
}

func (p *CaddyReverseProxy) GetActiveConnections() map[string]int {
	return make(map[string]int)
}
