// Package incus-adjacent: nomad provides a thin HTTP client for HashiCorp Nomad
// (https://developer.hashicorp.com/nomad/api-docs) so Forge can treat Nomad as a
// workload-orchestration runtime. It is configured entirely from the environment
// (NOMAD_ADDR / NOMAD_TOKEN) mirroring the Nomad CLI, and every method is
// nil-safe so the admin routes can always be registered.
package nomad

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

// RuntimeProvider is the Node.RuntimeProvider marker for Nomad-managed hosts.
// Nomad itself is a control-plane service reached over HTTP, so this is used by
// handlers only to label/filter nodes when a per-node target is requested.
const RuntimeProvider = "nomad"

// ErrNotConfigured reports that no Forge Orchestration control plane is
// configured; handlers answer 503 instead of rendering an empty cluster.
var ErrNotConfigured = errors.New("Forge Orchestration (driver: nomad) is not configured (set NOMAD_ADDR)")

// Job mirrors the subset of the Nomad job API shape (/v1/job, /v1/jobs).
type Job struct {
	Region            string            `json:"Region,omitempty"`
	Namespace         string            `json:"Namespace,omitempty"`
	ID                string            `json:"ID"`
	ParentID          string            `json:"ParentID,omitempty"`
	Name              string            `json:"Name"`
	Version           uint64            `json:"Version,omitempty"`
	Type              string            `json:"Type,omitempty"`
	Priority          int               `json:"Priority,omitempty"`
	Status            string            `json:"Status"`
	StatusDescription string            `json:"StatusDescription,omitempty"`
	SubmitTime        int64             `json:"SubmitTime,omitempty"`
	CreateTime        int64             `json:"CreateTime,omitempty"`
	UpdateTime        int64             `json:"UpdateTime,omitempty"`
	Meta              map[string]string `json:"Meta,omitempty"`
	JobModifyIndex    uint64            `json:"JobModifyIndex,omitempty"`
}

// Allocation mirrors the Nomad allocation API shape (/v1/allocation, /v1/job/…/allocations).
type Allocation struct {
	ID                string            `json:"ID"`
	Name              string            `json:"Name,omitempty"`
	NodeID            string            `json:"NodeID,omitempty"`
	JobID             string            `json:"JobID,omitempty"`
	TaskGroup         string            `json:"TaskGroup,omitempty"`
	Namespace         string            `json:"Namespace,omitempty"`
	ClientStatus      string            `json:"ClientStatus,omitempty"`
	ClientDescription string            `json:"ClientDescription,omitempty"`
	DesiredStatus     string            `json:"DesiredStatus,omitempty"`
	DesiredDescription string           `json:"DesiredDescription,omitempty"`
	DeployID          string            `json:"DeployID,omitempty"`
	CreatedAt         int64             `json:"CreatedAt,omitempty"`
	ModifiedAt        int64             `json:"ModifiedAt,omitempty"`
	JobModifyIndex    uint64            `json:"JobModifyIndex,omitempty"`
	AllocModifyIndex  uint64            `json:"AllocModifyIndex,omitempty"`
}

// Node mirrors a Nomad client node (/v1/node, list entries are NodeList).
type Node struct {
	ID           string            `json:"ID"`
	Name         string            `json:"Name,omitempty"`
	HTTPAddr     string            `json:"HTTPAddr,omitempty"`
	Attributes   map[string]string `json:"Attributes,omitempty"`
	NodeClass    string            `json:"NodeClass,omitempty"`
	NodePool     string            `json:"NodePool,omitempty"`
	Version      string            `json:"Version,omitempty"`
	Status       string            `json:"Status,omitempty"`
	StatusDescription string       `json:"StatusDescription,omitempty"`
	Drain        bool              `json:"Drain,omitempty"`
	Eligibility  string            `json:"Eligibility,omitempty"`
	CreatedAt    int64             `json:"CreatedAt,omitempty"`
	ModifiedAt   int64             `json:"ModifiedAt,omitempty"`
}

// NodeList is the compact node descriptor returned by GET /v1/nodes.
type NodeList struct {
	ID           string `json:"ID"`
	Name         string `json:"Name,omitempty"`
	HTTPAddr     string `json:"HTTPAddr,omitempty"`
	NodeClass    string `json:"NodeClass,omitempty"`
	NodePool     string `json:"NodePool,omitempty"`
	Version      string `json:"Version,omitempty"`
	Status       string `json:"Status,omitempty"`
	Drain        bool   `json:"Drain,omitempty"`
	Eligibility  string `json:"Eligibility,omitempty"`
}

// Deployment mirrors the Nomad deployment API shape (/v1/deployment).
type Deployment struct {
	ID                      string    `json:"ID"`
	JobID                   string    `json:"JobID,omitempty"`
	Namespace               string    `json:"Namespace,omitempty"`
	JobVersion              uint64    `json:"JobVersion,omitempty"`
	Status                  string    `json:"Status,omitempty"`
	StatusDescription       string    `json:"StatusDescription,omitempty"`
	DesiredStatus           string    `json:"DesiredStatus,omitempty"`
	DesiredCanonicalVersion string    `json:"DesiredCanonicalVersion,omitempty"`
	Canary                  bool      `json:"Canary,omitempty"`
	Pause                   bool      `json:"Pause,omitempty"`
	CreatedAt               int64     `json:"CreatedAt,omitempty"`
	ModifyIndex             uint64    `json:"ModifyIndex,omitempty"`
}

// Eval mirrors the Nomad evaluation API shape (/v1/evaluation).
type Eval struct {
	ID                string `json:"ID"`
	Priority          int    `json:"Priority,omitempty"`
	Type              string `json:"Type,omitempty"`
	Trigger           string `json:"Trigger,omitempty"`
	Namespace         string `json:"Namespace,omitempty"`
	JobID             string `json:"JobID,omitempty"`
	NodeID            string `json:"NodeID,omitempty"`
	Status            string `json:"Status,omitempty"`
	StatusDescription string `json:"StatusDescription,omitempty"`
	Wait              int64  `json:"Wait,omitempty"`
	CreatedAt         int64  `json:"CreatedAt,omitempty"`
	ModifiedAt        int64  `json:"ModifiedAt,omitempty"`
}

// Service is the Nomad HTTP client. Configuration is read from the environment
// (NOMAD_ADDR, NOMAD_TOKEN, NOMAD_REGION) at construction; it is nil-safe.
type Service struct {
	addr   string
	token  string
	region string
	logger *slog.Logger
	store  *store.Store
	client *http.Client
}

// New builds a Nomad service. db may be nil. Returns a service whose methods
// report "nomad not configured" until NOMAD_ADDR is set, so it is always safe to
// construct and pass into the HTTP Config.
func New(db *store.Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	addr := strings.TrimRight(strings.TrimSpace(os.Getenv("NOMAD_ADDR")), "/")
	if addr != "" && !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return &Service{
		addr:   addr,
		token:  strings.TrimSpace(os.Getenv("NOMAD_TOKEN")),
		region: strings.TrimSpace(os.Getenv("NOMAD_REGION")),
		logger: log,
		store:  db,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Available reports whether a Nomad control-plane address is configured.
func (s *Service) Available() bool {
	return s != nil && s.addr != ""
}

// do issues a request against the Nomad API. Nomad returns raw JSON (no sync
// envelope); the query parameters are always augmented with region when set.
func (s *Service) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	if s == nil || s.addr == "" {
		return ErrNotConfigured
	}
	target := s.addr + path
	if query == nil {
		query = url.Values{}
	}
	if s.region != "" && query.Get("region") == "" {
		query.Set("region", s.region)
	}
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode nomad request: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("X-Nomad-Token", s.token)
	}

	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("nomad request %s %s: %w", method, path, err)
	}
	defer res.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("nomad %s %s: status %d: %s", method, path, res.StatusCode, strings.TrimSpace(string(payload)))
	}
	// A truncated first page must never read as a complete listing.
	if res.Header.Get("X-Nomad-Result-Is-Truncated") == "true" {
		return fmt.Errorf("nomad %s %s: result set truncated by the server; retry with an explicit limit/prefix", method, path)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(payload, out)
}

// ListJobs returns every job (optionally scoped to a namespace).
func (s *Service) ListJobs(ctx context.Context, namespace string) ([]Job, error) {
	if s == nil {
		return nil, errors.New("Forge Orchestration service unavailable")
	}
	q := url.Values{}
	if namespace != "" {
		q.Set("namespace", namespace)
	}
	var out []Job
	if err := s.do(ctx, http.MethodGet, "/v1/jobs", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SubmitJob registers a job from its JSON specification document. The Nomad HTTP
// API consumes a job descriptor object; HCL must be rendered to JSON before it
// reaches this method (see the admin page notes).
func (s *Service) SubmitJob(ctx context.Context, job map[string]any) (*Job, error) {
	if s == nil {
		return nil, errors.New("Forge Orchestration service unavailable")
	}
	// Allow the caller to pass the raw job document as a string under an
	// "hcl"/"HCL" key. The Nomad HTTP API only accepts a JSON job descriptor, so
	// we decode JSON text here and reject genuine HCL with an actionable error.
	if raw, ok := job["hcl"].(string); ok && len(job) == 1 {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return nil, errors.New("nomad job must be a JSON job document; render HCL to JSON first (the HTTP API does not parse HCL)")
		}
		job = decoded
	} else if raw, ok := job["HCL"].(string); ok && len(job) == 1 {
		var decoded map[string]any
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			return nil, errors.New("nomad job must be a JSON job document; render HCL to JSON first (the HTTP API does not parse HCL)")
		}
		job = decoded
	}
	var out struct {
		JobID   string `json:"JobID"`
		EvalID  string `json:"EvalID"`
		Warnings string `json:"Warnings"`
	}
	if err := s.do(ctx, http.MethodPost, "/v1/jobs", nil, job, &out); err != nil {
		return nil, err
	}
	resolved := out.JobID
	if resolved == "" {
		if id, ok := job["ID"].(string); ok {
			resolved = id
		} else if name, ok := job["Name"].(string); ok {
			resolved = name
		}
	}
	return &Job{ID: resolved, Name: resolved}, nil
}

// StopJob stops (deregisters when purge=true) a job via PUT /v1/job/:id/stop.
func (s *Service) StopJob(ctx context.Context, jobID string, purge bool) (string, error) {
	if s == nil {
		return "", errors.New("Forge Orchestration service unavailable")
	}
	q := url.Values{}
	if purge {
		q.Set("purge", "true")
	}
	var out struct {
		EvalID string `json:"EvalID"`
	}
	if err := s.do(ctx, http.MethodPut, "/v1/job/"+url.PathEscape(jobID)+"/stop", q, nil, &out); err != nil {
		return "", err
	}
	return out.EvalID, nil
}

// GetJobStatus returns a single job's full definition.
func (s *Service) GetJobStatus(ctx context.Context, jobID string) (*Job, error) {
	if s == nil {
		return nil, errors.New("Forge Orchestration service unavailable")
	}
	var out Job
	if err := s.do(ctx, http.MethodGet, "/v1/job/"+url.PathEscape(jobID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAllocations returns a job's allocations (all jobs when jobID is empty).
func (s *Service) ListAllocations(ctx context.Context, jobID string) ([]Allocation, error) {
	if s == nil {
		return nil, errors.New("Forge Orchestration service unavailable")
	}
	path := "/v1/job/" + url.PathEscape(jobID) + "/allocations"
	if jobID == "" {
		path = "/v1/allocations"
	}
	var out []Allocation
	if err := s.do(ctx, http.MethodGet, path, url.Values{}, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListNodes returns every client node.
func (s *Service) ListNodes(ctx context.Context) ([]NodeList, error) {
	if s == nil {
		return nil, errors.New("Forge Orchestration service unavailable")
	}
	var out []NodeList
	if err := s.do(ctx, http.MethodGet, "/v1/nodes", url.Values{}, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetNode returns a single client node.
func (s *Service) GetNode(ctx context.Context, nodeID string) (*Node, error) {
	if s == nil {
		return nil, errors.New("Forge Orchestration service unavailable")
	}
	var out Node
	if err := s.do(ctx, http.MethodGet, "/v1/node/"+url.PathEscape(nodeID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DrainNode toggles drain mode on a client node. Nomad's drain endpoint is a PUT
// to /v1/node/:id/drain with a DrainSpec body; an empty body starts draining
// with defaults, and eligibility can be reversed by setting drain=false.
func (s *Service) DrainNode(ctx context.Context, nodeID string, drain bool) error {
	if s == nil {
		return errors.New("Forge Orchestration service unavailable")
	}
	q := url.Values{}
	if !drain {
		// "force-refuse" reversal is expressed by clearing eligibility.
		q.Set("no-delay", "true")
	}
	body := map[string]any{"drain": drain}
	if err := s.do(ctx, http.MethodPut, "/v1/node/"+url.PathEscape(nodeID)+"/drain", q, body, nil); err != nil {
		return err
	}
	return nil
}

// ListDeployments returns every deployment, newest handled first by the caller.
func (s *Service) ListDeployments(ctx context.Context) ([]Deployment, error) {
	if s == nil {
		return nil, errors.New("Forge Orchestration service unavailable")
	}
	var out []Deployment
	if err := s.do(ctx, http.MethodGet, "/v1/deployments", url.Values{}, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetDeployment returns a single deployment.
func (s *Service) GetDeployment(ctx context.Context, deploymentID string) (*Deployment, error) {
	if s == nil {
		return nil, errors.New("Forge Orchestration service unavailable")
	}
	var out Deployment
	if err := s.do(ctx, http.MethodGet, "/v1/deployment/"+url.PathEscape(deploymentID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PromoteAllocation promotes an allocation's canary deployment. It resolves the
// allocation's owning deployment then issues POST /v1/deployment/:id/promote.
func (s *Service) PromoteAllocation(ctx context.Context, allocID string) (string, error) {
	if s == nil {
		return "", errors.New("Forge Orchestration service unavailable")
	}
	var alloc Allocation
	if err := s.do(ctx, http.MethodGet, "/v1/allocation/"+url.PathEscape(allocID), nil, nil, &alloc); err != nil {
		return "", err
	}
	deployID := alloc.DeployID
	if deployID == "" {
		return "", fmt.Errorf("allocation %q has no active deployment to promote", allocID)
	}
	var out struct {
		EvalID string `json:"EvalID"`
	}
	if err := s.do(ctx, http.MethodPut, "/v1/deployment/"+url.PathEscape(deployID)+"/promote", url.Values{}, map[string]any{"Deployment": deployID}, &out); err != nil {
		return "", err
	}
	return deployID, nil
}
