package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

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
