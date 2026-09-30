package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// composeQueryTimeout bounds the two compose reads, which the node caps at 30s
// of its own (docker compose ps / logs). The panel's RPC default is exactly
// that long, so without a margin the panel hangs up at the same instant the
// node is about to answer — and dropping the connection is what cancels the
// node's request context, turning a slow answer into no answer.
const composeQueryTimeout = 60 * time.Second

type ComposeDeployRequest struct {
	StackID      string            `json:"stackId"`
	ComposeYAML  string            `json:"composeYaml"`
	EnvVars      map[string]string `json:"envVars,omitempty"`
	RegistryAuth []*RegistryAuth   `json:"registryAuth,omitempty"`
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

// composeStackPath builds the node route for a stack, refusing an ID the node
// would not accept rather than sending it and letting a differently-shaped
// failure come back. It mirrors Beacon's validStackID exactly: the two tiers
// must disagree about nothing here, or a stack the panel thinks exists cannot
// be addressed at all.
func composeStackPath(stackID, action string) (string, error) {
	if err := requireComposeStackID(stackID); err != nil {
		return "", err
	}
	path := "/compose/" + stackID
	if action != "" {
		path += "/" + action
	}
	return path, nil
}

// requireComposeStackID refuses an ID that cannot address one stack.
func requireComposeStackID(stackID string) error {
	if !validComposeStackID(stackID) {
		return fmt.Errorf("invalid compose stack id %q", stackID)
	}
	return nil
}

// validComposeStackID is Beacon's rule (beacon/internal/server/compose.go
// validStackID): first character [a-z0-9], remainder [a-z0-9-_], at most 128
// bytes. A '/' or ".." could rewrite the route being signed, which is why the
// ID is validated before it is interpolated instead of escaped afterwards.
func validComposeStackID(stackID string) bool {
	if stackID == "" || len(stackID) > 128 {
		return false
	}
	if stackID[0] < 'a' || stackID[0] > 'z' {
		if stackID[0] < '0' || stackID[0] > '9' {
			return false
		}
	}
	for _, r := range stackID {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// composeFailureError keeps the node's own answer. Beacon reports a failed
// compose run as 409 with the docker output in the body, and that output is the
// only record of why the stack did not come up, so it goes on the typed error
// the same way daemonResponseError would have put it.
func composeFailureError(operation string, res *http.Response, raw []byte) error {
	return &ResponseError{
		Operation:  operation,
		StatusCode: res.StatusCode,
		Details:    strings.TrimSpace(string(raw)),
	}
}

func (c *Client) ComposeDeploy(ctx context.Context, baseURL, nodeToken string, req ComposeDeployRequest) (ComposeDeployResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return ComposeDeployResponse{}, err
	}
	if err := validStackIDForError(req.StackID); err != nil {
		return ComposeDeployResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/compose/deploy"
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return ComposeDeployResponse{}, err
	}
	// The node allows 10 minutes for `docker compose up`; the RPC default would
	// cut it at 30s and cancel the deploy that is still running.
	res, err := c.WithTimeout(longTransferTimeout).httpClient.Do(httpReq)
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
	path, err := composeStackPath(stackID, "pull")
	if err != nil {
		return ComposePullResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + path
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, nil)
	if err != nil {
		return ComposePullResponse{}, err
	}
	res, err := c.WithTimeout(longTransferTimeout).httpClient.Do(httpReq)
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
	path, err := composeStackPath(stackID, "status")
	if err != nil {
		return ComposeStatusResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + path
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return ComposeStatusResponse{}, err
	}
	res, err := c.WithTimeout(composeQueryTimeout).httpClient.Do(httpReq)
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
	path, err := composeStackPath(stackID, "logs")
	if err != nil {
		return "", err
	}
	// A non-positive tail is "not specified", not "zero lines": sending
	// tail=0 would ask the node for nothing and read the empty answer back as
	// a stack that produced no log output.
	endpoint := fmt.Sprintf("%s%s", strings.TrimRight(baseURL, "/"), path)
	if tail > 0 {
		endpoint += fmt.Sprintf("?tail=%d", tail)
	}
	if service != "" {
		if tail > 0 {
			endpoint += "&service=" + url.QueryEscape(service)
		} else {
			endpoint += "?service=" + url.QueryEscape(service)
		}
	}
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Accept", "text/plain")
	res, err := c.WithTimeout(composeQueryTimeout).httpClient.Do(httpReq)
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
	path, err := composeStackPath(stackID, action)
	if err != nil {
		return ComposeOperationResponse{}, err
	}
	operation := "compose " + action
	if action == "" {
		operation = "compose delete"
	}
	endpoint := strings.TrimRight(baseURL, "/") + path
	httpReq, err := c.newRequest(ctx, nodeToken, method, endpoint, nil)
	if err != nil {
		return ComposeOperationResponse{}, err
	}
	// The node allows 5 minutes for compose down/up of a single service, and a
	// panel-side hangup cancels it mid-run.
	res, err := c.WithTimeout(longTransferTimeout).httpClient.Do(httpReq)
	if err != nil {
		return ComposeOperationResponse{}, err
	}
	defer res.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(res.Body, 1*1024*1024))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Nothing was performed as asked, so nothing is returned as done: the
		// caller gets the zero value plus the node's own output.
		if readErr != nil && len(raw) == 0 {
			return ComposeOperationResponse{}, daemonResponseError(operation, res)
		}
		return ComposeOperationResponse{}, composeFailureError(operation, res, raw)
	}
	if readErr != nil {
		return ComposeOperationResponse{}, readErr
	}
	var payload ComposeOperationResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ComposeOperationResponse{}, fmt.Errorf("decode %s response: %w", operation, err)
	}
	return payload, nil
}

// validStackIDForError adapts the route guard for bodies that carry the ID
// instead of the path.
func validStackIDForError(stackID string) error {
	if !validComposeStackID(stackID) {
		return fmt.Errorf("invalid compose stack id %q", stackID)
	}
	return nil
}
