package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (c *Client) GetFirewallStatus(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	return c.hostGet(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/firewall/status")
}

func (c *Client) EnableFirewall(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/firewall/enable", nil)
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
		return nil, fmt.Errorf("enable firewall failed with status %d", res.StatusCode)
	}
	var payload json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) DisableFirewall(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/firewall/disable", nil)
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
		return nil, fmt.Errorf("disable firewall failed with status %d", res.StatusCode)
	}
	var payload json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) ListFirewallRules(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	return c.hostGet(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/firewall/rules")
}

func (c *Client) AddFirewallRule(ctx context.Context, baseURL, nodeToken string, body interface{}) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.hostPostJSON(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/firewall/rules", payload)
}

func (c *Client) DeleteFirewallRule(ctx context.Context, baseURL, nodeToken, ruleID string) error {
	req, err := c.newRequest(ctx, nodeToken, http.MethodDelete, strings.TrimRight(baseURL, "/")+"/v1/firewall/rules/"+ruleID, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("delete firewall rule failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) UpdateFirewallRule(ctx context.Context, baseURL, nodeToken, ruleID string, body interface{}) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, nodeToken, http.MethodPut, strings.TrimRight(baseURL, "/")+"/v1/firewall/rules/"+ruleID, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("update firewall rule failed with status %d", res.StatusCode)
	}
	var result json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) OpenFirewallPort(ctx context.Context, baseURL, nodeToken string, body interface{}) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.hostPostJSON(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/firewall/port", payload)
}

func (c *Client) ListPortForwards(ctx context.Context, baseURL, nodeToken string) (json.RawMessage, error) {
	return c.hostGet(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/firewall/forward")
}

func (c *Client) AddPortForward(ctx context.Context, baseURL, nodeToken string, body interface{}) (json.RawMessage, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return c.hostPostJSON(ctx, nodeToken, strings.TrimRight(baseURL, "/")+"/v1/firewall/forward", payload)
}

func (c *Client) DeletePortForward(ctx context.Context, baseURL, nodeToken, forwardID string) error {
	req, err := c.newRequest(ctx, nodeToken, http.MethodDelete, strings.TrimRight(baseURL, "/")+"/v1/firewall/forward/"+forwardID, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("delete port forward failed with status %d", res.StatusCode)
	}
	return nil
}

func (c *Client) hostPostJSON(ctx context.Context, nodeToken, url string, payload []byte) (json.RawMessage, error) {
	req, err := c.newRequest(ctx, nodeToken, http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("host post request failed with status %d: %s", res.StatusCode, tryReadBody(res.Body))
	}
	var result json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}
