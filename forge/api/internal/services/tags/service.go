// Package tags implements Forge's color-coded tag system: a global catalog of
// labels (e.g. "production", "maintenance-window") that can be assigned to
// applications, servers, and environments. Tags drive dashboard filtering,
// grouping, and bulk operations ("stop every server tagged X").
//
// The layering mirrors the domains package: this package owns the domain types
// and all validation/business rules; persistence lives in internal/store (which
// imports this package for the types) and is reached only through the Repository
// interface below. HTTP handlers talk to Service, never to the store directly.
package tags

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ResourceType enumerates the kinds of objects a tag can be attached to.
type ResourceType string

const (
	ResourceApplication ResourceType = "application"
	ResourceServer      ResourceType = "server"
	ResourceEnvironment ResourceType = "environment"
)

var knownResourceTypes = map[ResourceType]bool{
	ResourceApplication: true,
	ResourceServer:      true,
	ResourceEnvironment: true,
}

// ValidResourceType reports whether t is an assignable resource kind.
func ValidResourceType(t string) bool { return knownResourceTypes[ResourceType(t)] }

// ResourceTypes returns the assignable resource kinds in a stable order.
func ResourceTypes() []string {
	return []string{string(ResourceApplication), string(ResourceServer), string(ResourceEnvironment)}
}

// Tag is a named, color-coded label. Color is stored as a 6-digit hex string
// ("#rrggbb"); the frontend renders it via a sanitized inline style.
type Tag struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
}

// TaggedResource is one resource carrying a given tag. GetResourcesByTag returns
// a mixed list (across resource kinds) so bulk operations can iterate and each
// entry self-identifies its kind for dispatch.
type TaggedResource struct {
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	Name         string `json:"name"`
}

// CreateRequest carries the fields for a new tag.
type CreateRequest struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// UpdateRequest carries patchable tag fields (nil = leave unchanged).
type UpdateRequest struct {
	Name        *string `json:"name"`
	Color       *string `json:"color"`
	Description *string `json:"description"`
}

// Sentinel errors surface honest, actionable failures to the HTTP layer.
var (
	ErrNotFound            = errors.New("tag not found")
	ErrDuplicateName       = errors.New("a tag with this name already exists")
	ErrInvalidResourceType = errors.New("unknown resource type (expected application, server, or environment)")
	ErrEmptyName           = errors.New("tag name is required")
	ErrNameTooLong         = errors.New("tag name must be 64 characters or fewer")
	ErrInvalidColor        = errors.New("tag color must be a 6-digit hex value like #1a2b3c")
	ErrEmptyResourceID     = errors.New("resource id is required")
)

const maxNameLen = 64

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// DefaultColor is applied when a create request omits a color.
const DefaultColor = "#6366f1"

// Repository is the persistence surface the service depends on. *store.Store
// implements it; keeping it an interface here preserves the dependency
// direction (service does not import store).
type Repository interface {
	CreateTag(ctx context.Context, name, color, description string) (Tag, error)
	UpdateTag(ctx context.Context, id string, req UpdateRequest) (Tag, error)
	DeleteTag(ctx context.Context, id string) error
	ListTags(ctx context.Context) ([]Tag, error)
	GetTag(ctx context.Context, id string) (Tag, error)
	AssignTag(ctx context.Context, resourceType, resourceID, tagID string) error
	UnassignTag(ctx context.Context, resourceType, resourceID, tagID string) error
	ListTagsForResource(ctx context.Context, resourceType, resourceID string) ([]Tag, error)
	ListResourcesByTag(ctx context.Context, tagID string) ([]TaggedResource, error)
	ResourceExists(ctx context.Context, resourceType, resourceID string) (bool, error)
	TagExists(ctx context.Context, tagID string) (bool, error)
}

// Service exposes tag CRUD, assignment, and bulk-target resolution.
type Service struct {
	repo Repository
}

// New builds the tag service over a repository.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// normalizeName trims and case-folds a tag name for storage. Names are stored
// lowercased so uniqueness and matching are case-insensitive.
func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// normalizeColor validates and lowercases a hex color, applying the default for
// an empty value. It fails closed on malformed input.
func normalizeColor(color string) (string, error) {
	color = strings.TrimSpace(color)
	if color == "" {
		return DefaultColor, nil
	}
	if !hexColorRe.MatchString(color) {
		return "", ErrInvalidColor
	}
	return strings.ToLower(color), nil
}

// CreateTag validates the request and persists a new tag.
func (s *Service) CreateTag(ctx context.Context, req CreateRequest) (Tag, error) {
	name := normalizeName(req.Name)
	if name == "" {
		return Tag{}, ErrEmptyName
	}
	if len(name) > maxNameLen {
		return Tag{}, ErrNameTooLong
	}
	color, err := normalizeColor(req.Color)
	if err != nil {
		return Tag{}, err
	}
	return s.repo.CreateTag(ctx, name, color, strings.TrimSpace(req.Description))
}

// UpdateTag applies a patch to an existing tag.
func (s *Service) UpdateTag(ctx context.Context, id string, req UpdateRequest) (Tag, error) {
	if req.Name != nil {
		name := normalizeName(*req.Name)
		if name == "" {
			return Tag{}, ErrEmptyName
		}
		if len(name) > maxNameLen {
			return Tag{}, ErrNameTooLong
		}
		req.Name = &name
	}
	if req.Color != nil {
		color, err := normalizeColor(*req.Color)
		if err != nil {
			return Tag{}, err
		}
		req.Color = &color
	}
	if req.Description != nil {
		desc := strings.TrimSpace(*req.Description)
		req.Description = &desc
	}
	return s.repo.UpdateTag(ctx, id, req)
}

// DeleteTag removes a tag and (via cascade) all of its assignments.
func (s *Service) DeleteTag(ctx context.Context, id string) error {
	return s.repo.DeleteTag(ctx, id)
}

// ListTags returns the full tag catalog, ordered by name.
func (s *Service) ListTags(ctx context.Context) ([]Tag, error) {
	list, err := s.repo.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

// GetTag returns a single tag.
func (s *Service) GetTag(ctx context.Context, id string) (Tag, error) {
	return s.repo.GetTag(ctx, id)
}

// AssignTag attaches a tag to a resource. Both the tag and the target resource
// must exist; assignment fails closed otherwise. Assignments are idempotent.
func (s *Service) AssignTag(ctx context.Context, resourceType, resourceID, tagID string) error {
	if !ValidResourceType(resourceType) {
		return ErrInvalidResourceType
	}
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return ErrEmptyResourceID
	}
	ok, err := s.repo.TagExists(ctx, tagID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	ok, err = s.repo.ResourceExists(ctx, resourceType, resourceID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return s.repo.AssignTag(ctx, resourceType, resourceID, tagID)
}

// UnassignTag detaches a tag from a resource.
func (s *Service) UnassignTag(ctx context.Context, resourceType, resourceID, tagID string) error {
	if !ValidResourceType(resourceType) {
		return ErrInvalidResourceType
	}
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return ErrEmptyResourceID
	}
	return s.repo.UnassignTag(ctx, resourceType, resourceID, tagID)
}

// GetResourceTags returns the tags currently attached to a resource.
func (s *Service) GetResourceTags(ctx context.Context, resourceType, resourceID string) ([]Tag, error) {
	if !ValidResourceType(resourceType) {
		return nil, ErrInvalidResourceType
	}
	list, err := s.repo.ListTagsForResource(ctx, resourceType, strings.TrimSpace(resourceID))
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	return list, nil
}

// GetResourcesByTag returns every resource carrying the tag, optionally filtered
// to a single resource type (empty = all types). The result backs bulk
// operations: callers iterate and dispatch an action per resource.
func (s *Service) GetResourcesByTag(ctx context.Context, tagID, resourceType string) ([]TaggedResource, error) {
	if resourceType != "" && !ValidResourceType(resourceType) {
		return nil, ErrInvalidResourceType
	}
	resources, err := s.repo.ListResourcesByTag(ctx, tagID)
	if err != nil {
		return nil, err
	}
	if resourceType == "" {
		return resources, nil
	}
	filtered := make([]TaggedResource, 0, len(resources))
	for _, r := range resources {
		if r.ResourceType == resourceType {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}
