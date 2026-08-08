package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type ComposeDeployRequest struct {
	StackID     string            `json:"stackId"`
	ComposeYAML string            `json:"composeYaml"`
	EnvVars     map[string]string `json:"envVars,omitempty"`
}

type ComposeDeployResponse struct {
	StackID string `json:"stackId"`
	Output  string `json:"output,omitempty"`
}

type ComposeOperationResponse struct {
	StackID string `json:"stackId"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

type ComposeServiceState struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
	State  string `json:"state"`
	Ports  string `json:"ports"`
}

type ComposeStatusResponse struct {
	StackID  string                `json:"stackId"`
	Services []ComposeServiceState `json:"services"`
}

type ComposePullResponse struct {
	StackID string `json:"stackId"`
	Output  string `json:"output,omitempty"`
}

func (c *Client) ComposeDeploy(ctx context.Context, baseURL, nodeToken string, req ComposeDeployRequest) (ComposeDeployResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return ComposeDeployResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/compose/deploy"
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return ComposeDeployResponse{}, err
	}
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ComposeDeployResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ComposeDeployResponse{}, daemonResponseError("compose deploy", res)
	}
	var payload ComposeDeployResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return ComposeDeployResponse{}, err
	}
	return payload, nil
}

func (c *Client) ComposeStop(ctx context.Context, baseURL, nodeToken, stackID string) (ComposeOperationResponse, error) {
	return c.composeAction(ctx, baseURL, nodeToken, stackID, "stop", http.MethodPost)
}

func (c *Client) ComposeStart(ctx context.Context, baseURL, nodeToken, stackID string) (ComposeOperationResponse, error) {
	return c.composeAction(ctx, baseURL, nodeToken, stackID, "start", http.MethodPost)
}

func (c *Client) ComposeRestart(ctx context.Context, baseURL, nodeToken, stackID string) (ComposeOperationResponse, error) {
	return c.composeAction(ctx, baseURL, nodeToken, stackID, "restart", http.MethodPost)
}

func (c *Client) ComposeDelete(ctx context.Context, baseURL, nodeToken, stackID string) (ComposeOperationResponse, error) {
	return c.composeAction(ctx, baseURL, nodeToken, stackID, "", http.MethodDelete)
}

func (c *Client) ComposePull(ctx context.Context, baseURL, nodeToken, stackID string) (ComposePullResponse, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/compose/" + stackID + "/pull"
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, nil)
	if err != nil {
		return ComposePullResponse{}, err
	}
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ComposePullResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ComposePullResponse{}, daemonResponseError("compose pull", res)
	}
	var payload ComposePullResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return ComposePullResponse{}, err
	}
	return payload, nil
}

func (c *Client) ComposeStatus(ctx context.Context, baseURL, nodeToken, stackID string) (ComposeStatusResponse, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/compose/" + stackID + "/status"
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return ComposeStatusResponse{}, err
	}
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ComposeStatusResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ComposeStatusResponse{}, daemonResponseError("compose status", res)
	}
	var payload ComposeStatusResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return ComposeStatusResponse{}, err
	}
	return payload, nil
}

func (c *Client) ComposeLogs(ctx context.Context, baseURL, nodeToken, stackID, service string, tail int) (string, error) {
	endpoint := fmt.Sprintf("%s/compose/%s/logs?tail=%d",
		strings.TrimRight(baseURL, "/"), stackID, tail)
	if service != "" {
		endpoint += "&service=" + url.QueryEscape(service)
	}
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Accept", "text/plain")
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", daemonResponseError("compose logs", res)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1*1024*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) composeAction(ctx context.Context, baseURL, nodeToken, stackID, action string, method string) (ComposeOperationResponse, error) {
	path := "/compose/" + stackID
	if action != "" {
		path += "/" + action
	}
	endpoint := strings.TrimRight(baseURL, "/") + path
	httpReq, err := c.newRequest(ctx, nodeToken, method, endpoint, nil)
	if err != nil {
		return ComposeOperationResponse{}, err
	}
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ComposeOperationResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var payload ComposeOperationResponse
		if decodeErr := json.NewDecoder(res.Body).Decode(&payload); decodeErr == nil {
			return payload, daemonResponseError("compose "+action, res)
		}
		return ComposeOperationResponse{}, daemonResponseError("compose "+action, res)
	}
	var payload ComposeOperationResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return ComposeOperationResponse{}, err
	}
	return payload, nil
}
