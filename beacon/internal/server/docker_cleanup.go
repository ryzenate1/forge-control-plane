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
	"log"
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

	// ContainerCountKnown is false when the engine answered "-1" for how many
	// containers reference this image: it did not count. InUse then reports the
	// safe assumption (in use) rather than a measurement, so an uncounted image
	// is never treated as a deletable one.
	ContainerCountKnown bool `json:"containerCountKnown,omitempty"`
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

	// SizeIncompleteCount records how many objects the engine returned without
	// ever calculating a size for them. Their bytes are missing from the totals
	// above rather than measured as zero, so a small total can be told apart from
	// a node that genuinely holds little data.
	SizeIncompleteCount int `json:"sizeIncompleteCount,omitempty"`

	// UnattributedImageBytes is the part of the engine's own LayersSize total that
	// no per-image entry accounts for. The panel prunes by image, so bytes sitting
	// outside the reported breakdown are not reclaimable by any listed candidate.
	UnattributedImageBytes int64 `json:"unattributedImageBytes,omitempty"`

	// AccountingNotes are gaps found while assembling this report ("volume size
	// not calculated", ...). They ride along so a partial report is never read as
	// a complete one.
	AccountingNotes []string `json:"accountingNotes,omitempty"`
}

// dockerCleanupPruneResult mirrors dockerleanup.PruneResult (nodeId is stamped
// by the panel). ReclaimedBytes is what the engine freed; RemovedCount is how
// many objects were deleted.
type dockerCleanupPruneResult struct {
	ReclaimedBytes int64 `json:"reclaimedBytes"`
	RemovedCount   int   `json:"removedCount"`

	// FailedCount / SkippedCount / Errors describe the objects that were NOT
	// deleted. A prune that removed 2 of 10 images is not the same result as one
	// that removed 10, so failures are reported instead of dropped; Errors carries
	// at most the first few reasons while FailedCount stays exact.
	FailedCount  int      `json:"failedCount,omitempty"`
	SkippedCount int      `json:"skippedCount,omitempty"`
	Errors       []string `json:"errors,omitempty"`

	// ReclaimedBytesKnown is false when the engine could not be asked for image
	// sizes, or an image disappeared before it could be measured. ReclaimedBytes
	// is then 0 meaning "not measured", not "nothing was freed".
	ReclaimedBytesKnown bool `json:"reclaimedBytesKnown"`
}

// dockerCleanupErrorDetailLimit caps how many per-object failure strings a prune
// response carries; FailedCount is the complete count regardless.
const dockerCleanupErrorDetailLimit = 12

func (r *dockerCleanupPruneResult) recordFailure(reason string, skipped bool) {
	r.FailedCount++
	if skipped {
		r.SkippedCount++
	}
	if len(r.Errors) < dockerCleanupErrorDetailLimit {
		r.Errors = append(r.Errors, reason)
	}
}

// isImageInUseError recognises the engine's "this image is being used by a
// container" refusal, which is a skip (the image is legitimately still wanted)
// rather than an unexpected failure.
func isImageInUseError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "image is being used") ||
		strings.Contains(msg, "is using referenced image") ||
		strings.Contains(msg, "conflict")
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

	var imageBytesSum int64
	for _, img := range du.Images {
		if img == nil {
			continue
		}
		// image.Summary.Containers is documented as "-1 = not set / calculated".
		// The panel prunes everything it is told is not in use, so an unknown
		// count must fail closed and be reported as in use: "the engine did not
		// tell us" cannot become "safe to delete".
		containerCountKnown := img.Containers >= 0
		inUse := img.Containers != 0
		if !containerCountKnown {
			out.SizeIncompleteCount++
		}
		if img.Size < 0 {
			out.SizeIncompleteCount++
		} else {
			imageBytesSum += img.Size
		}
		out.Images = append(out.Images, dockerCleanupImageInfo{
			ID:                  img.ID,
			RepoTags:            img.RepoTags,
			Size:                img.Size,
			CreatedAt:           time.Unix(img.Created, 0).UTC().Format(time.RFC3339),
			InUse:               inUse,
			ContainerCountKnown: containerCountKnown,
		})
	}
	// LayersSize is the engine's own total. Anything it counts that no per-image
	// entry accounts for is reported separately instead of vanishing from the
	// report — otherwise a node can look smaller than it is.
	if du.LayersSize > imageBytesSum {
		out.UnattributedImageBytes = du.LayersSize - imageBytesSum
		out.AccountingNotes = append(out.AccountingNotes,
			"engine reports more image layers than the per-image breakdown accounts for")
	}

	for _, c := range du.Containers {
		if c == nil {
			continue
		}
		if c.SizeRootFs < 0 {
			out.SizeIncompleteCount++
			out.AccountingNotes = append(out.AccountingNotes,
				"container root filesystem size was not calculated; containersBytes is a partial total")
			continue
		}
		out.ContainersBytes += c.SizeRootFs
	}
	for _, v := range du.Volumes {
		if v == nil {
			continue
		}
		if v.UsageData == nil {
			// No usage data at all is "size unknown", not "volume is empty".
			out.SizeIncompleteCount++
			out.AccountingNotes = append(out.AccountingNotes,
				"volume usage data was not reported; volumesBytes is a partial total")
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
	// does not return per-image byte counts. If the list fails the removals still
	// proceed, but the reclaimed total is reported as not measured rather than as
	// zero, because "0 bytes freed" and "we could not tell" are different answers.
	sizeByID := map[string]int64{}
	res := dockerCleanupPruneResult{ReclaimedBytesKnown: true}
	images, err := docker.ImageList(r.Context(), image.ListOptions{All: true})
	if err != nil {
		res.ReclaimedBytesKnown = false
		res.recordFailure("image size listing failed, reclaimed bytes not measured: "+err.Error(), false)
	} else {
		for _, img := range images {
			sizeByID[img.ID] = img.Size
		}
	}

	for _, id := range ids {
		if _, err := docker.ImageRemove(r.Context(), id, image.RemoveOptions{Force: false, PruneChildren: true}); err != nil {
			// A still-in-use image is not a hard failure of the whole batch: skip it
			// so the panel can prune the rest. Both the skip and any other error are
			// counted and carried in the response, so a batch that deleted nothing
			// cannot read back as a successful prune.
			res.recordFailure(id+": "+err.Error(), isImageInUseError(err))
			continue
		}
		res.RemovedCount++
		size, known := sizeByID[id]
		if !known {
			// The image was not in the size listing (or vanished from it): its bytes
			// are unknown, not zero.
			res.ReclaimedBytesKnown = false
			continue
		}
		res.ReclaimedBytes += size
	}
	if res.RemovedCount == 0 && res.FailedCount > 0 {
		log.Printf("docker prune-images removed nothing: %d of %d images failed or were skipped", res.FailedCount, len(ids))
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
		ReclaimedBytes:      int64(report.SpaceReclaimed),
		RemovedCount:        len(report.CachesDeleted),
		ReclaimedBytesKnown: true,
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
		ReclaimedBytes:      int64(report.SpaceReclaimed),
		RemovedCount:        len(report.VolumesDeleted),
		ReclaimedBytesKnown: true,
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
