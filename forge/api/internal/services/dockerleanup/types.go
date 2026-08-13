// Package dockerleanup implements per-node Docker disk-usage reporting and
// automated cleanup. It is inspired by Dokploy's docker-disk-usage router
// (getDiskUsage / getBuildCache / pruneBuildCache) and CapRover's
// DiskCleanupManager (scheduled pruning of unused images with a configurable
// retention count), adapted to Forge's API -> Beacon split: the API owns
// policy state and the scheduler; Beacon executes the actual Docker queries
// and prune commands against its local engine over the signed admin channel.
//
// Disk accounting is performed by the Docker SDK's DiskUsage() call on each
// node; policy evaluation (which images are safe to remove while preserving
// the N most recent deployed versions) is decided here in the control plane so
// the retention rule is enforced consistently even if a node is down.
package dockerleanup

import (
	"context"
	"net/http"
	"time"
)

// ImageInfo is one image present on a node. InUse is derived from the engine:
// an image with at least one (running or stopped) container referencing it, or
// a container's current image, is considered in use and never pruned.
type ImageInfo struct {
	ID        string    `json:"id"`
	RepoTags  []string  `json:"tags"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
	InUse     bool      `json:"inUse"`
}

// DiskUsage is the aggregate disk accounting for a single node. Sizes are in
// bytes. TotalBytes is the sum of the four categories (images + containers +
// volumes + build cache) and is what the UI shows as the headline number.
type DiskUsage struct {
	NodeID          string      `json:"nodeId"`
	NodeName        string      `json:"nodeName"`
	ImagesBytes     int64       `json:"imagesBytes"`
	ContainersBytes int64       `json:"containersBytes"`
	VolumesBytes    int64       `json:"volumesBytes"`
	BuildCacheBytes int64       `json:"buildCacheBytes"`
	TotalBytes      int64       `json:"totalBytes"`
	Images          []ImageInfo `json:"images"`
}

// PruneResult reports what a single prune action reclaimed. ReclaimedBytes is
// the space the engine freed; RemovedCount is how many objects were deleted.
type PruneResult struct {
	NodeID         string `json:"nodeId"`
	ReclaimedBytes int64  `json:"reclaimedBytes"`
	RemovedCount   int    `json:"removedCount"`
}

// CleanupPolicy is a scheduled cleanup rule. NodeID == "" means the policy is
// global and applies to every reachable node; otherwise it is host-scoped.
// MostRecentLimit is the retention floor: that many most-recently-created
// deployed images are always preserved so a prune cannot strand the version a
// workload currently runs. LastStatus/LastError capture the most recent run so
// the admin UI can surface failures without a separate run-history table.
type CleanupPolicy struct {
	ID              string     `json:"id"`
	NodeID          string     `json:"nodeId,omitempty"`
	Schedule        string     `json:"schedule"`
	MostRecentLimit int        `json:"mostRecentLimit"`
	Enabled         bool       `json:"enabled"`
	PruneBuildCache bool       `json:"pruneBuildCache"`
	PruneVolumes    bool       `json:"pruneVolumes"`
	LastRunAt       *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt       *time.Time `json:"nextRunAt,omitempty"`
	LastStatus      string     `json:"lastStatus,omitempty"`
	LastError       string     `json:"lastError,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	CreatedBy       string     `json:"createdBy,omitempty"`
}

// CreatePolicyInput carries the fields to register a new policy. NodeID is
// optional (empty == global). MostRecentLimit defaults to 1 when zero so the
// currently-deployed image is never removed by an accidental misconfiguration.
type CreatePolicyInput struct {
	NodeID          string
	Schedule        string
	MostRecentLimit int
	Enabled         *bool
	PruneBuildCache *bool
	PruneVolumes    *bool
	CreatedBy       string
}

// UpdatePolicyInput uses pointer patch semantics so the caller can distinguish
// "leave unchanged" from "set to zero/false". NextRunAt is recomputed by the
// service whenever the schedule or enabled flag changes.
type UpdatePolicyInput struct {
	NodeID          *string
	Schedule        *string
	MostRecentLimit *int
	Enabled         *bool
	PruneBuildCache *bool
	PruneVolumes    *bool
}

// NodeTarget is the resolved control address + shared secret for one node. The
// http layer builds these from the node store; the service never touches the
// node credential directly, it only forwards the pair to the Beacon signer.
type NodeTarget struct {
	ID    string
	Name  string
	URL   string
	Token string
}

// Beacon signs an outbound admin request the same way daemon.Client does. Only
// the two primitives needed to talk to Beacon's docker-cleanup endpoints are
// exposed, which keeps the service decoupled from the concrete daemon client
// and lets tests supply a fake. *daemon.Client satisfies this interface.
type Beacon interface {
	// SignedHeaders returns the HMAC auth headers for method+requestURI over
	// body (body may be nil for GET). requestURI must be exactly the path and
	// query string that will be requested (i.e. url.URL.RequestURI()).
	SignedHeaders(nodeToken, method, requestURI string, body []byte) (http.Header, error)
	// HTTPClient returns the client used to perform the request.
	HTTPClient() *http.Client
}

// NodeResolver lets the scheduler find where to run a policy. A global policy
// fans out across All(); a host-scoped policy resolves a single node.
type NodeResolver interface {
	All(ctx context.Context) ([]NodeTarget, error)
	Resolve(ctx context.Context, nodeID string) (NodeTarget, error)
}
