package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (c *Client) hostGet(ctx context.Context, nodeToken, url string) (json.RawMessage, error) {
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("host request failed with status %d", res.StatusCode)
	}
	var payload json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) GetHostInfo(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	return c.hostGet(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/host/info")
}

func (c *Client) GetHostDisk(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	return c.hostGet(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/host/disk")
}

func (c *Client) GetHostMemory(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	return c.hostGet(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/host/memory")
}

func (c *Client) GetHostNetwork(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	return c.hostGet(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/host/network")
}

func (c *Client) GetHostProcesses(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	return c.hostGet(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/host/processes")
}
