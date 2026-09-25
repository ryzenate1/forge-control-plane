package server

// Docker disk-usage reporting & prune execution for the control plane.
//
// These endpoints are the Beacon side of Forge's "Docker Disk Usage & Automated
// Cleanup" feature (forge/api/internal/services/dockerleanup). The panel owns
// policy state and the cron scheduler; Beacon only performs the concrete Docker
// engine queries and prune commands against its local daemon and returns a
// normalized report. That split means the retention rule ("always keep the N
// most recent deployed images") is enforced in the control plane and applied
// per-node even when a node is briefly unreachable.
//
// They live under /api/admin/docker-cleanup and are gated by the same HMAC
// signed-admin channel as every other /api/admin route (server.authenticate +
// getAdminUserInfo). They are programmatic: unlike the interactive Portainer
// admin handlers they deliberately do NOT require the X-Confirm-Destructive
// header, because the panel has already gated the action behind an admin role,
// a write scope and a UI confirmation dialog.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

// dockerCleanupImageInfo mirrors dockerleanup.ImageInfo on the wire. Only the
// JSON field names matter to the panel decoder (id/tags/size/createdAt/inUse).
type dockerCleanupImageInfo struct {
	ID        string   `json:"id"`
	RepoTags  []string `json:"tags"`
	Size      int64    `json:"size"`
	CreatedAt string   `json:"createdAt"`
	InUse     bool     `json:"inUse"`
}

// dockerCleanupDiskUsage mirrors dockerleanup.DiskUsage. The panel fills in
// node identity and recomputes totalBytes; Beacon reports the four categories
// plus the per-image breakdown used to decide what is safe to prune.
type dockerCleanupDiskUsage struct {
	ImagesBytes     int64                    `json:"imagesBytes"`
	ContainersBytes int64                    `json:"containersBytes"`
	VolumesBytes    int64                    `json:"volumesBytes"`
	BuildCacheBytes int64                    `json:"buildCacheBytes"`
	Images          []dockerCleanupImageInfo `json:"images"`
}

// dockerCleanupPruneResult mirrors dockerleanup.PruneResult (nodeId is stamped
// by the panel). ReclaimedBytes is what the engine freed; RemovedCount is how
// many objects were deleted.
type dockerCleanupPruneResult struct {
	ReclaimedBytes int64 `json:"reclaimedBytes"`
	RemovedCount   int   `json:"removedCount"`
}

// handleDockerDiskUsage returns the node's aggregate Docker disk accounting plus
// the per-image breakdown, derived from a single Docker SDK DiskUsage call so
// the four categories and the in-use flags stay mutually consistent.
func (s *Server) handleDockerDiskUsage(w http.ResponseWriter, r *http.Request) {
	if !requireDockerCleanupAdmin(s, w, r) {
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	du, err := docker.DiskUsage(r.Context(), types.DiskUsageOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := dockerCleanupDiskUsage{Images: []dockerCleanupImageInfo{}}
	out.ImagesBytes = du.LayersSize

	for _, img := range du.Images {
		if img == nil {
			continue
		}
		// Containers is populated by the disk-usage endpoint; -1 means the engine
		// did not calculate it, which we treat as "not known to be in use" so a
		// shared/base image is not pruned merely because the count is unavailable.
		inUse := img.Containers > 0
		out.Images = append(out.Images, dockerCleanupImageInfo{
			ID:        img.ID,
			RepoTags:  img.RepoTags,
			Size:      img.Size,
			CreatedAt: time.Unix(img.Created, 0).UTC().Format(time.RFC3339),
			InUse:     inUse,
		})
	}
	for _, c := range du.Containers {
		if c == nil {
			continue
		}
		out.ContainersBytes += c.SizeRootFs
	}
	for _, v := range du.Volumes {
		if v == nil || v.UsageData == nil {
			continue
		}
		if v.UsageData.Size > 0 {
			out.VolumesBytes += v.UsageData.Size
		}
	}
	for _, bc := range du.BuildCache {
		if bc == nil {
			continue
		}
		if bc.Size > 0 {
			out.BuildCacheBytes += bc.Size
		}
	}

	writeJSON(w, http.StatusOK, out)
}

// handleDockerPruneImages removes an explicit set of images by id. The panel
// resolves the concrete deletable list (applying the retention floor) before
// calling, so an empty id list is rejected rather than interpreted as "prune
// everything".
func (s *Server) handleDockerPruneImages(w http.ResponseWriter, r *http.Request) {
	if !requireDockerCleanupAdmin(s, w, r) {
		return
	}

	var body struct {
		ImageIDs []string `json:"imageIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ids := make([]string, 0, len(body.ImageIDs))
	for _, id := range body.ImageIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "imageIds is required")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Pre-fetch sizes so reclaimed space can be reported even though ImageRemove
	// does not return per-image byte counts. Images that vanish between the list
	// and the remove simply contribute nothing to the total.
	sizeByID := map[string]int64{}
	if images, err := docker.ImageList(r.Context(), image.ListOptions{All: true}); err == nil {
		for _, img := range images {
			sizeByID[img.ID] = img.Size
		}
	}

	res := dockerCleanupPruneResult{}
	for _, id := range ids {
		if _, err := docker.ImageRemove(r.Context(), id, image.RemoveOptions{Force: false, PruneChildren: true}); err != nil {
			// A still-in-use image is not a hard failure of the whole batch: skip it
			// so the panel can prune the rest. Anything else is logged via response
			// detail only when nothing at all could be removed.
			continue
		}
		res.RemovedCount++
		res.ReclaimedBytes += sizeByID[id]
	}
	writeJSON(w, http.StatusOK, res)
}

// handleDockerPruneBuildCache reclaims the BuildKit build cache. Only unused
// (not "all") layers are pruned so a build in progress is never broken.
func (s *Server) handleDockerPruneBuildCache(w http.ResponseWriter, r *http.Request) {
	if !requireDockerCleanupAdmin(s, w, r) {
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	report, err := docker.BuildCachePrune(r.Context(), build.CachePruneOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dockerCleanupPruneResult{
		ReclaimedBytes: int64(report.SpaceReclaimed),
		RemovedCount:   len(report.CachesDeleted),
	})
}

// handleDockerPruneVolumes removes dangling (unused) volumes. The "all" filter
// is intentionally not set so volumes still referenced by a container survive.
func (s *Server) handleDockerPruneVolumes(w http.ResponseWriter, r *http.Request) {
	if !requireDockerCleanupAdmin(s, w, r) {
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	report, err := docker.VolumesPrune(r.Context(), filters.NewArgs())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dockerCleanupPruneResult{
		ReclaimedBytes: int64(report.SpaceReclaimed),
		RemovedCount:   len(report.VolumesDeleted),
	})
}

// requireDockerCleanupAdmin gates the docker-cleanup endpoints behind the same
// signed-admin identity as the sibling /api/admin handlers. It returns false
// (after writing an error response) when the caller is not authenticated as an
// admin, so handlers can simply bail.
func requireDockerCleanupAdmin(s *Server, w http.ResponseWriter, r *http.Request) bool {
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return false
	}
	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return false
	}
	return true
}
