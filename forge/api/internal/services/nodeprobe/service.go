package nodeprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

// Service probes a node's beacon daemon over HTTPS with HMAC signature,
// returning the daemon's reported system information. This mirrors
// the `GET /api/system` endpoint that the admin panel polls to show
// live daemon status.
type Service struct {
	store   *store.Store
	client  *http.Client
	signer  *daemon.Client
	timeout time.Duration
}

func NewService(s *store.Store) *Service {
	return &Service{
		store:   s,
		client:  &http.Client{Timeout: 5 * time.Second},
		signer:  daemon.NewClient(),
		timeout: 5 * time.Second,
	}
}

// NodeSystemInformation is the JSON shape returned by `GET /api/system` on the
// beacon (and the JSON we return from the panel-side proxy).
type NodeSystemInformation struct {
	NodeID          string   `json:"nodeId"`
	Online          bool     `json:"online"`
	Version         string   `json:"version,omitempty"`
	OS              string   `json:"os,omitempty"`
	Architecture    string   `json:"architecture,omitempty"`
	CPUThreads      int      `json:"cpuThreads,omitempty"`
	MemoryMB        uint64   `json:"memoryMb,omitempty"`
	DockerStatus    string   `json:"dockerStatus,omitempty"`
	DockerAvailable bool     `json:"dockerAvailable"`
	Capabilities    []string `json:"capabilities,omitempty"`
	UptimeSeconds   int64    `json:"uptimeSeconds,omitempty"`
	FetchedAt       string   `json:"fetchedAt"`
	Error           string   `json:"error,omitempty"`
}

// ProbeNode fetches the system information from the node's beacon and returns
// a structured result. If the node or its URL is unreachable, the returned
// struct has `Online=false` and an `Error` field.
func (s *Service) ProbeNode(ctx context.Context, nodeID string) (NodeSystemInformation, error) {
	info := NodeSystemInformation{NodeID: nodeID, FetchedAt: time.Now().UTC().Format(time.RFC3339)}
	if s.store == nil {
		info.Online = false
		info.Error = "no database connection"
		return info, nil
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if err != nil {
		info.Online = false
		info.Error = err.Error()
		return info, nil
	}
	scheme := strings.TrimSpace(node.Scheme)
	if scheme == "" {
		scheme = "https"
	}
	fqdn := strings.TrimSpace(node.FQDN)
	if fqdn == "" {
		info.Online = false
		info.Error = "node has no FQDN configured"
		return info, nil
	}
	endpoint := fmt.Sprintf("%s://%s/api/system", scheme, fqdn)

	// Build an HMAC-signed request with this node's current credential.
	token, err := s.store.GetNodeDaemonCredential(ctx, node.ID)
	if err != nil {
		info.Online = false
		info.Error = err.Error()
		return info, nil
	}
	if token == "" {
		info.Online = false
		info.Error = "node has no daemon token configured"
		return info, nil
	}
	method := http.MethodGet
	uri := "/api/system"
	headers, err := s.signer.SignedHeaders(token, method, uri, nil)
	if err != nil {
		info.Online = false
		info.Error = err.Error()
		return info, nil
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		info.Online = false
		info.Error = err.Error()
		return info, nil
	}
	req.Header = headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Forge-NodeProbe/1.0")

	res, err := s.client.Do(req)
	if err != nil {
		info.Online = false
		info.Error = err.Error()
		return info, nil
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		info.Online = false
		info.Error = fmt.Sprintf("daemon returned %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
		return info, nil
	}
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		info.Online = false
		info.Error = "invalid JSON from daemon: " + err.Error()
		return info, nil
	}
	info.NodeID = nodeID
	info.Online = true
	info.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	if info.DockerStatus == "ok" {
		info.DockerAvailable = true
	}
	return info, nil
}
