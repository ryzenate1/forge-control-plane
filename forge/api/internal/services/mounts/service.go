// Package mounts implements declarative, per-application persistent storage:
// named volumes, host bind mounts, tmpfs, and DB-stored seed files that survive
// redeploys (Dokploy mount management + Dokku persistent-storage inspired).
//
// Layering matches the tags package: this package owns the AppMount domain type,
// all fail-closed validation, and compose injection; persistence lives in
// internal/store (which imports this package for the type) behind the Repository
// interface. HTTP handlers talk to Service only.
package mounts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

// MountType enumerates the supported persistent storage kinds.
type MountType string

const (
	MountVolume   MountType = "volume"    // Docker named volume, survives redeploys
	MountBind     MountType = "bind"      // host path bind mount
	MountTmpfs    MountType = "tmpfs"     // ephemeral in-memory mount
	MountSeedFile MountType = "seed-file" // DB-stored file materialized at deploy time
)

func validType(t string) bool {
	switch MountType(t) {
	case MountVolume, MountBind, MountTmpfs, MountSeedFile:
		return true
	default:
		return false
	}
}

// AppMount is one declarative mount for an application.
type AppMount struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"applicationId"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Source        string    `json:"source"`
	Target        string    `json:"target"`
	ReadOnly      bool      `json:"readOnly"`
	Content       *string   `json:"content,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// CreateRequest carries fields for a new mount.
type CreateRequest struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Source   string  `json:"source"`
	Target   string  `json:"target"`
	ReadOnly bool    `json:"readOnly"`
	Content  *string `json:"content"`
}

// UpdateRequest carries patchable mount fields (nil = leave unchanged).
type UpdateRequest struct {
	Name     *string `json:"name"`
	Type     *string `json:"type"`
	Source   *string `json:"source"`
	Target   *string `json:"target"`
	ReadOnly *bool   `json:"readOnly"`
	Content  *string `json:"content"`
}

// Sentinel errors surfaced to the HTTP layer.
var (
	ErrNotFound        = errors.New("mount not found")
	ErrInvalidType     = errors.New("mount type must be one of: volume, bind, tmpfs, seed-file")
	ErrMissingName     = errors.New("mount name is required")
	ErrMissingTarget   = errors.New("mount target path is required")
	ErrDangerousTarget = errors.New("mount target is a reserved/dangerous container path")
	ErrBadPath         = errors.New("mount path must be an absolute, clean path without '..'")
	ErrBindSource      = errors.New("bind mount requires a valid absolute host source path")
	ErrVolumeSource    = errors.New("volume source must be a valid docker volume name")
	ErrSeedContent     = errors.New("seed-file mount requires non-empty content")
	ErrSeedTooLarge    = errors.New("seed-file content exceeds the maximum allowed size")
	ErrEmptyAppID      = errors.New("application id is required")
)

// MaxSeedContentBytes bounds how large a stored seed file may be (64 KiB).
const MaxSeedContentBytes = 64 * 1024

var (
	volumeNameRe   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
	dangerousExact = map[string]bool{
		"/": true, "/bin": true, "/sbin": true, "/lib": true, "/lib64": true,
		"/boot": true, "/dev": true, "/proc": true, "/sys": true, "/etc": true,
		"/usr": true, "/var": true, "/root": true, "/etc/passwd": true,
		"/etc/shadow": true, "/etc/sudoers": true, "/etc/hosts": true,
		"/etc/group": true, "/var/run/docker.sock": true, "/host": true,
	}
	// dangerousPrefixes block container paths whose contents must never be
	// shadowed by a user mount (subdirectories of an otherwise-fine parent).
	dangerousPrefixes = []string{"/bin/", "/sbin/", "/lib/", "/lib64/", "/proc/", "/sys/", "/dev/", "/etc/", "/boot/"}
)

// Repository is the persistence port for mounts.
type Repository interface {
	CreateAppMount(ctx context.Context, appID string, req CreateRequest) (AppMount, error)
	UpdateAppMount(ctx context.Context, mountID string, req UpdateRequest) (AppMount, error)
	DeleteAppMount(ctx context.Context, mountID string) error
	ListAppMounts(ctx context.Context, appID string) ([]AppMount, error)
	GetAppMount(ctx context.Context, mountID string) (AppMount, error)
	AppOrgID(ctx context.Context, appID string) (string, error)
	UserInOrg(ctx context.Context, orgID, userID string) (bool, error)
}

// Service exposes mount CRUD, validation, and compose application.
type Service struct {
	repo Repository
}

// New builds the mount service.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// Validate checks a fully-resolved mount definition and fails closed. It is
// exported so the HTTP layer can give the UI path-validation feedback without
// duplicating the rules.
func Validate(req CreateRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return ErrMissingName
	}
	if !validType(req.Type) {
		return ErrInvalidType
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		return ErrMissingTarget
	}
	if err := validateContainerTarget(target); err != nil {
		return err
	}
	switch MountType(req.Type) {
	case MountBind:
		source := strings.TrimSpace(req.Source)
		if source == "" {
			return ErrBindSource
		}
		if err := validateBindSource(source); err != nil {
			return err
		}
	case MountVolume:
		source := strings.TrimSpace(req.Source)
		if source != "" && !volumeNameRe.MatchString(source) {
			return ErrVolumeSource
		}
	case MountSeedFile:
		if req.Content == nil || strings.TrimSpace(*req.Content) == "" {
			return ErrSeedContent
		}
		if len(*req.Content) > MaxSeedContentBytes {
			return ErrSeedTooLarge
		}
	}
	return nil
}

// validateContainerTarget rejects absolute-path traversal and reserved system
// paths that a user must never mount over.
func validateContainerTarget(target string) error {
	if !path.IsAbs(target) || path.Clean(target) != target {
		return ErrBadPath
	}
	if strings.Contains(target, "..") {
		return ErrBadPath
	}
	if dangerousExact[target] {
		return ErrDangerousTarget
	}
	for _, prefix := range dangerousPrefixes {
		if strings.HasPrefix(target, prefix) {
			return ErrDangerousTarget
		}
	}
	return nil
}

// validateBindSource enforces the host-side allowlist / denylist model shared
// with the admin mount store: when MOUNTS_ALLOWED_PREFIX is set, only sources
// under those prefixes are permitted; otherwise a denylist blocks sensitive host
// paths (fail-closed).
func validateBindSource(source string) error {
	if !path.IsAbs(source) || path.Clean(source) != source || strings.Contains(source, "..") || strings.Contains(source, "\\") {
		return ErrBindSource
	}
	if allowed := allowedPrefixes(); len(allowed) > 0 {
		for _, prefix := range allowed {
			if source == prefix || strings.HasPrefix(source, prefix+"/") {
				return nil
			}
		}
		return fmt.Errorf("bind source %q is not under an allowed host prefix (%s)", source, strings.Join(allowed, ", "))
	}
	blocked := []string{"/", "/etc", "/proc", "/sys", "/dev", "/boot", "/root", "/bin", "/sbin", "/lib", "/usr", "/var/run", "/run", "/var/lib/docker", "/var/lib/forge"}
	for _, b := range blocked {
		if source == b || strings.HasPrefix(source, b+"/") {
			return fmt.Errorf("bind source %q is a protected host path", source)
		}
	}
	return nil
}

func allowedPrefixes() []string {
	raw := strings.TrimSpace(os.Getenv("MOUNTS_ALLOWED_PREFIX"))
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, ";", ",")
	raw = strings.ReplaceAll(raw, ":", ",")
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\t' })
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." {
			continue
		}
		cleaned := path.Clean(p)
		if cleaned != "." {
			out = append(out, cleaned)
		}
	}
	return out
}

// Create validates then persists a new mount for an application.
func (s *Service) Create(ctx context.Context, appID string, req CreateRequest) (AppMount, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return AppMount{}, ErrEmptyAppID
	}
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	if err := Validate(req); err != nil {
		return AppMount{}, err
	}
	return s.repo.CreateAppMount(ctx, appID, req)
}

// Update validates the merged result and persists the patch.
func (s *Service) Update(ctx context.Context, mountID string, req UpdateRequest) (AppMount, error) {
	mountID = strings.TrimSpace(mountID)
	if mountID == "" {
		return AppMount{}, ErrNotFound
	}
	existing, err := s.repo.GetAppMount(ctx, mountID)
	if err != nil {
		return AppMount{}, err
	}
	merged := mergeForValidation(existing, req)
	if err := Validate(merged); err != nil {
		return AppMount{}, err
	}
	if req.Type != nil {
		t := strings.ToLower(strings.TrimSpace(*req.Type))
		req.Type = &t
	}
	if req.Target != nil {
		t := strings.TrimSpace(*req.Target)
		req.Target = &t
	}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" {
			return AppMount{}, ErrMissingName
		}
		req.Name = &n
	}
	return s.repo.UpdateAppMount(ctx, mountID, req)
}

// Delete removes a mount.
func (s *Service) Delete(ctx context.Context, mountID string) error {
	return s.repo.DeleteAppMount(ctx, strings.TrimSpace(mountID))
}

// List returns an application's mounts.
func (s *Service) List(ctx context.Context, appID string) ([]AppMount, error) {
	return s.repo.ListAppMounts(ctx, strings.TrimSpace(appID))
}

// Get returns a single mount.
func (s *Service) Get(ctx context.Context, mountID string) (AppMount, error) {
	return s.repo.GetAppMount(ctx, strings.TrimSpace(mountID))
}

// CanManageApp reports whether the user may manage an application's mounts:
// admins pass; otherwise the app's org must contain the user.
func (s *Service) CanManageApp(ctx context.Context, appID, userID string, isAdmin bool) (bool, error) {
	if isAdmin {
		return true, nil
	}
	orgID, err := s.repo.AppOrgID(ctx, strings.TrimSpace(appID))
	if err != nil {
		return false, err
	}
	if orgID == "" {
		return false, nil
	}
	return s.repo.UserInOrg(ctx, orgID, userID)
}

func mergeForValidation(m AppMount, req UpdateRequest) CreateRequest {
	out := CreateRequest{
		Name:     m.Name,
		Type:     m.Type,
		Source:   m.Source,
		Target:   m.Target,
		ReadOnly: m.ReadOnly,
		Content:  m.Content,
	}
	if req.Name != nil {
		out.Name = *req.Name
	}
	if req.Type != nil {
		out.Type = *req.Type
	}
	if req.Source != nil {
		out.Source = *req.Source
	}
	if req.Target != nil {
		out.Target = *req.Target
	}
	if req.ReadOnly != nil {
		out.ReadOnly = *req.ReadOnly
	}
	if req.Content != nil {
		out.Content = req.Content
	}
	return out
}

// ---- Compose injection ----------------------------------------------------

// SeedServiceName derives a stable, collision-resistant service name for the
// init/sidecar container that materializes a seed-file mount into a volume.
func SeedServiceName(appID, mountID string) string {
	sum := sha256.Sum256([]byte(appID + ":" + mountID))
	return "forge-seed-" + hex.EncodeToString(sum[:])[:16]
}

// SeedVolumeName derives the named volume a seed sidecar writes into.
func SeedVolumeName(appID, mountID string) string {
	sum := sha256.Sum256([]byte(appID + ":vol:" + mountID))
	return "forge_seed_" + hex.EncodeToString(sum[:])[:16]
}

// volumeFor returns the compose volume source for a mount: the explicit named
// volume, or a deterministic per-mount volume when none was provided.
func volumeFor(m AppMount) string {
	if strings.TrimSpace(m.Source) != "" {
		return m.Source
	}
	sum := sha256.Sum256([]byte(m.ApplicationID + ":" + m.ID + ":" + m.Name))
	return "forge_" + hex.EncodeToString(sum[:])[:16]
}

// renderSeedCommand builds the shell command the seed sidecar runs to write the
// stored content into the mounted volume path. Content is passed via a heredoc
// with a generated delimiter so arbitrary file bytes are preserved verbatim.
func renderSeedCommand(targetInVolume, content string) string {
	delim := "FORGE_SEED_EOF"
	for strings.Contains(content, delim) {
		delim += "_"
	}
	return fmt.Sprintf(
		"set -euo pipefail; mkdir -p \"$(dirname '%s')\"; cat > '%s' <<'%s'\n%s\n%s\n",
		targetInVolume, targetInVolume, delim, content, delim,
	)
}

// ApplyToCompose injects an application's declarative mounts into a parsed
// compose document (map[string]any, as produced by yaml.Unmarshal). Named
// volumes, bind mounts, tmpfs, and seed-file sidecars are added idempotently.
// It mutates composeDoc in place. The method name matches the requested spec; a
// context is threaded first so the mount list can be loaded from the store.
func (s *Service) ApplyToCompose(ctx context.Context, appID string, composeDoc map[string]any) error {
	if composeDoc == nil {
		return errors.New("compose document is nil")
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return ErrEmptyAppID
	}
	list, err := s.repo.ListAppMounts(ctx, appID)
	if err != nil {
		return err
	}
	return ApplyMountsToCompose(composeDoc, list)
}

// ApplyMountsToCompose is the pure, side-effect injection used by ApplyToCompose.
// It is exported so callers that already hold a mount list (e.g. the deploy
// pipeline) can reuse the exact same rules without a store round-trip.
func ApplyMountsToCompose(composeDoc map[string]any, list []AppMount) error {
	if composeDoc == nil {
		return errors.New("compose document is nil")
	}
	if len(list) == 0 {
		return nil
	}
	services := asMap(composeDoc["services"])
	if services == nil {
		services = map[string]any{}
		composeDoc["services"] = services
	}
	topVolumes := asMap(composeDoc["volumes"])
	if topVolumes == nil {
		topVolumes = map[string]any{}
		composeDoc["volumes"] = topVolumes
	}

	// Services we must NOT decorate with the app's mounts (our own seed sidecars).
	synthetic := map[string]bool{}

	for _, m := range list {
		switch MountType(m.Type) {
		case MountTmpfs:
			for name, svc := range services {
				if synthetic[name] {
					continue
				}
				appendUnique(svc, "tmpfs", m.Target)
			}
		case MountBind:
			spec := bindSpec(m)
			for name, svc := range services {
				if synthetic[name] {
					continue
				}
				appendUnique(svc, "volumes", spec)
			}
		case MountVolume:
			vol := volumeFor(m)
			topVolumes[vol] = map[string]any{}
			spec := vol + ":" + m.Target + roSuffix(m.ReadOnly)
			for name, svc := range services {
				if synthetic[name] {
					continue
				}
				appendUnique(svc, "volumes", spec)
			}
		case MountSeedFile:
			vol := SeedVolumeName(m.ApplicationID, m.ID)
			sidecar := SeedServiceName(m.ApplicationID, m.ID)
			synthetic[sidecar] = true
			topVolumes[vol] = map[string]any{}
			// Seed sidecar writes the stored content into the shared volume once.
			services[sidecar] = map[string]any{
				"image":      "alpine:3.21",
				"command":    []string{"sh", "-c", renderSeedCommand("/seed/"+path.Base(m.Target), derefContent(m.Content))},
				"volumes":    []any{vol + ":/seed"},
				"restart":    "no",
				"entrypoint": []string{"sh", "-c"},
			}
			// The real services read the file from the populated volume via subPath.
			spec := map[string]any{
				"type":     "volume",
				"source":   vol,
				"target":   m.Target,
				"read_only": true,
				"volume":   map[string]any{"subpath": path.Base(m.Target)},
			}
			for name, svc := range services {
				if synthetic[name] {
					continue
				}
				appendUniqueLong(svc, "volumes", spec)
			}
		}
	}
	return nil
}

func bindSpec(m AppMount) string {
	return m.Source + ":" + m.Target + roSuffix(m.ReadOnly)
}

func roSuffix(readOnly bool) string {
	if readOnly {
		return ":ro"
	}
	return ""
}

func derefContent(c *string) string {
	if c == nil {
		return ""
	}
	return *c
}

// asMap coerces a loosely-typed compose node into a mutable map, tolerating the
// map[any]any shape some YAML decoders produce.
func asMap(v any) map[string]any {
	switch t := v.(type) {
	case map[string]any:
		return t
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = val
		}
		return out
	default:
		return nil
	}
}

// appendUnique adds a short-syntax entry (string) to a service's named list
// unless an equivalent value already exists, so repeated deploys are idempotent.
func appendUnique(service any, key, value string) {
	svc := asMap(service)
	if svc == nil {
		return
	}
	list := asList(svc[key])
	for _, item := range list {
		if asString(item) == value {
			return
		}
	}
	svc[key] = append(list, value)
}

// appendUniqueLong adds a long-syntax mount (map) to a service's volumes list,
// skipping an exact target collision.
func appendUniqueLong(service any, key string, value map[string]any) {
	svc := asMap(service)
	if svc == nil {
		return
	}
	target := asString(value["target"])
	list := asList(svc[key])
	for _, item := range list {
		if im := asMap(item); im != nil && asString(im["target"]) == target {
			return
		}
		if asString(item) == target {
			return
		}
	}
	svc[key] = append(list, value)
}

func asList(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case nil:
		return nil
	default:
		return []any{v}
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
