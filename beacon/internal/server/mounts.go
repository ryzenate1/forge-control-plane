package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gamepanel/beacon/internal/rootfs"
	"gamepanel/beacon/internal/runtime"
)

type mountConfiguration struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

func (s *Server) runtimeMounts(mounts []mountConfiguration) ([]runtime.Mount, error) {
	if len(mounts) == 0 {
		return nil, nil
	}

	allowed := s.allowedMountSources()
	result := make([]runtime.Mount, 0, len(mounts))
	for _, configured := range mounts {
		source, err := allowedMountSource(configured.Source, allowed)
		if err != nil {
			return nil, err
		}
		target := path.Clean(strings.TrimSpace(configured.Target))
		if !path.IsAbs(target) || target == "." {
			return nil, fmt.Errorf("mount target %q must be an absolute container path", configured.Target)
		}
		if target == "/home/container" {
			return nil, errors.New("custom mount cannot replace /home/container")
		}
		result = append(result, runtime.Mount{Source: source, Target: target, ReadOnly: configured.ReadOnly})
	}
	// Record every mount source Beacon actually provisioned so cleanup can
	// refuse paths it never created. The record is the confinement boundary
	// for the destructive endpoint below.
	s.mountRecordsMu.Lock()
	if s.mountRecords == nil {
		s.mountRecords = make(map[string]struct{})
	}
	for _, m := range result {
		s.mountRecords[m.Source] = struct{}{}
	}
	s.mountRecordsMu.Unlock()
	return result, nil
}

func (s *Server) runtimeMountsFromConfiguration(payload map[string]any) ([]runtime.Mount, error) {
	raw, ok := payload["mounts"]
	if !ok {
		return nil, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode mounts: %w", err)
	}
	var mounts []mountConfiguration
	if err := json.Unmarshal(encoded, &mounts); err != nil {
		return nil, fmt.Errorf("decode mounts: %w", err)
	}
	return s.runtimeMounts(mounts)
}

func allowedMountSource(source string, allowed []string) (string, error) {
	if len(allowed) == 0 {
		return "", errors.New("custom mounts are not enabled on this node")
	}
	if !filepath.IsAbs(source) {
		return "", fmt.Errorf("mount source %q must be an absolute host path", source)
	}
	canonicalSource, err := filepath.EvalSymlinks(filepath.Clean(source))
	if err != nil {
		return "", fmt.Errorf("resolve mount source %q: %w", source, err)
	}
	for _, permitted := range allowed {
		if !filepath.IsAbs(permitted) {
			continue
		}
		canonicalPermitted, err := filepath.EvalSymlinks(filepath.Clean(permitted))
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(canonicalPermitted, canonicalSource)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return canonicalSource, nil
		}
	}
	return "", fmt.Errorf("mount source %q is not within configured allowed_mounts", source)
}

func (s *Server) reconcileRuntimeConfiguration(ctx context.Context, desired runtime.CreateRequest) error {
	existing, err := s.runtime.Inspect(ctx, desired.ServerID)
	if err != nil {
		return fmt.Errorf("inspect existing workload: %w", err)
	}
	if !existing.Exists {
		return s.runtime.Create(ctx, desired)
	}
	reconciler, ok := s.runtime.(runtime.Reconciler)
	if !ok {
		provider := "configured"
		if named, ok := s.runtime.(interface{ Provider() string }); ok {
			provider = named.Provider()
		}
		return fmt.Errorf("runtime %q cannot reconcile an existing workload", provider)
	}
	return reconciler.Reconcile(ctx, desired)
}

type mountCleanupRequest struct {
	Source string `json:"source"`
}

func (s *Server) cleanupMount(w http.ResponseWriter, r *http.Request) {
	var body mountCleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	// Destructive by design: require explicit confirmation like every other
	// destructive admin endpoint.
	if r.Header.Get("X-Confirm-Destructive") != "true" {
		http.Error(w, "destructive operation requires X-Confirm-Destructive: true header", http.StatusBadRequest)
		return
	}
	source := strings.TrimSpace(body.Source)
	if source == "" {
		http.Error(w, "source is required", http.StatusBadRequest)
		return
	}
	if !filepath.IsAbs(source) {
		http.Error(w, "source must be an absolute host path", http.StatusBadRequest)
		return
	}
	allowed := s.allowedMountSources()
	if len(allowed) == 0 {
		http.Error(w, "custom mounts are not enabled on this node", http.StatusBadRequest)
		return
	}
	canonical, err := allowedMountSource(source, allowed)
	if err != nil {
		if !isPathNotExist(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !mountSourceWithinAllowed(source, allowed) {
			http.Error(w, fmt.Sprintf("mount source %q is not within configured allowed_mounts", source), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": false, "reason": "directory does not exist"})
		return
	}
	permittedRoot, relative, err := allowedMountRelative(canonical, allowed)
	if err != nil || relative == "" || relative == "." {
		http.Error(w, "refusing to remove an allowed mount root", http.StatusBadRequest)
		return
	}
	// Restrict cleanup to Beacon records: only mount sources Beacon actually
	// provisioned through runtimeMounts may be removed here. Anything else is
	// an arbitrary host path that happens to sit under an allowed root.
	s.mountRecordsMu.RLock()
	_, recorded := s.mountRecords[canonical]
	s.mountRecordsMu.RUnlock()
	if !recorded {
		http.Error(w, "mount source is not a Beacon-managed record", http.StatusForbidden)
		return
	}
	confined, err := rootfs.New(permittedRoot)
	if err != nil {
		http.Error(w, "failed to open mount root", http.StatusInternalServerError)
		return
	}
	defer confined.Close()
	info, statErr := confined.Stat(filepath.ToSlash(relative))
	if statErr != nil {
		if os.IsNotExist(statErr) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": false, "reason": "directory does not exist"})
			return
		}
		http.Error(w, statErr.Error(), http.StatusInternalServerError)
		return
	}
	_ = info // RemoveAll safely handles regular files and directories.
	if err := confined.RemoveAll(filepath.ToSlash(relative)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Audit the destructive action. The panel HMAC already authenticated the
	// caller; the audit trail records who removed a Beacon-managed mount
	// source and which one, so an unexpected cleanup is attributable.
	s.logAdminAction(r.Context(), mountCleanupActor(r), "mount:cleanup", "mount", &canonical, map[string]interface{}{"source": canonical, "root": permittedRoot, "relative": relative})
	s.mountRecordsMu.Lock()
	delete(s.mountRecords, canonical)
	s.mountRecordsMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": true})
}

// mountCleanupActor attributes the cleanup in the audit log. Panel-HMAC calls
// carry no JWT user, so they are recorded as the node operator channel.
func mountCleanupActor(r *http.Request) string {
	if user := strings.TrimSpace(r.Header.Get("X-Panel-User-ID")); user != "" {
		return user
	}
	return "beacon-admin"
}

func isPathNotExist(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return false
}

func allowedMountRelative(source string, allowed []string) (string, string, error) {
	for _, permitted := range allowed {
		canonicalPermitted, err := filepath.EvalSymlinks(filepath.Clean(permitted))
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(canonicalPermitted, source)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return canonicalPermitted, relative, nil
		}
	}
	return "", "", errors.New("mount source is outside allowed roots")
}

func mountSourceWithinAllowed(source string, allowed []string) bool {
	if !filepath.IsAbs(source) {
		return false
	}
	cleanSource := filepath.Clean(source)
	for _, permitted := range allowed {
		if !filepath.IsAbs(permitted) {
			continue
		}
		cleanPermitted := filepath.Clean(permitted)
		relative, err := filepath.Rel(cleanPermitted, cleanSource)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
