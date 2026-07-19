package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestRegionCRUD(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	// ListRegions empty
	regions, err := s.ListRegions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 0 {
		t.Fatalf("expected 0 regions, got %d", len(regions))
	}

	// CreateRegion
	req := CreateRegionRequest{
		Name:        "US East",
		Slug:        "  US__East / 1  ",
		Description: "US East Coast region",
		Enabled:     true,
	}
	region, err := s.CreateRegion(ctx, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if region.Name != "US East" {
		t.Fatalf("name = %q, want %q", region.Name, "US East")
	}
	if region.Slug != "us-east-1" {
		t.Fatalf("slug = %q, want %q", region.Slug, "us-east-1")
	}
	if region.Description != "US East Coast region" {
		t.Fatalf("description = %q, want %q", region.Description, "US East Coast region")
	}
	if !region.Enabled {
		t.Fatal("expected enabled = true")
	}
	if region.NodeCount != 0 {
		t.Fatalf("expected node count 0, got %d", region.NodeCount)
	}
	if region.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	// GetRegion
	got, err := s.GetRegion(ctx, region.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != region.ID {
		t.Fatalf("GetRegion id = %q, want %q", got.ID, region.ID)
	}

	// GetRegion not found
	_, err = s.GetRegion(ctx, uuid.NewString())
	if err == nil {
		t.Fatal("expected error for non-existent region")
	}

	// UpdateRegion
	updated, err := s.UpdateRegion(ctx, region.ID, UpdateRegionRequest{
		Name:        "US East 2",
		Slug:        "us-east-2",
		Description: "Updated region",
		Enabled:     false,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "US East 2" {
		t.Fatalf("updated name = %q, want %q", updated.Name, "US East 2")
	}
	if updated.Slug != "us-east-2" {
		t.Fatalf("updated slug = %q, want %q", updated.Slug, "us-east-2")
	}

	// UpdateRegion not found
	_, err = s.UpdateRegion(ctx, uuid.NewString(), UpdateRegionRequest{
		Name: "Nope", Slug: "nope", Description: "", Enabled: true,
	}, nil)
	if err == nil {
		t.Fatal("expected error updating non-existent region")
	}

	// CreateRegion with empty name fails
	_, err = s.CreateRegion(ctx, CreateRegionRequest{Name: "", Slug: "", Description: "", Enabled: true}, nil)
	if err == nil {
		t.Fatal("expected error for empty name")
	}

	// CreateRegion with invalid slug fails
	_, err = s.CreateRegion(ctx, CreateRegionRequest{Name: "Bad", Slug: "  ", Description: "", Enabled: true}, nil)
	if err == nil {
		t.Fatal("expected error for whitespace-only slug")
	}

	// ListRegions with count
	regions, err = s.ListRegions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 {
		t.Fatalf("expected 1 region, got %d", len(regions))
	}
	if regions[0].NodeCount != 0 {
		t.Fatalf("expected 0 nodes, got %d", regions[0].NodeCount)
	}

	// DeleteRegion success
	if err := s.DeleteRegion(ctx, region.ID, nil); err != nil {
		t.Fatal(err)
	}

	// Verify deleted
	_, err = s.GetRegion(ctx, region.ID)
	if err == nil {
		t.Fatal("expected error for deleted region")
	}

	// DeleteRegion not found
	err = s.DeleteRegion(ctx, uuid.NewString(), nil)
	if err == nil {
		t.Fatal("expected error deleting non-existent region")
	}
}

func TestRegionDeleteWithAttachedNodes(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	locationID := uuid.NewString()
	regionID := uuid.NewString()
	nodeID := uuid.NewString()

	if _, err := s.db.Exec(ctx, `INSERT INTO locations (id, short, long) VALUES ($1, 'test', 'Test')`, locationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO regions (id, uuid, name, slug) VALUES ($1, $1, 'Test Region', 'test-region')`, regionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO nodes (id, name, region, base_url, token_hash, location_id, region_id)
		VALUES ($1, 'Test Node', 'test', 'https://node.test', 'hash', $2, $3)
	`, nodeID, locationID, regionID); err != nil {
		t.Fatal(err)
	}

	err := s.DeleteRegion(ctx, regionID, nil)
	if err == nil {
		t.Fatal("expected error deleting region with attached node")
	}
}

func TestRegionSlugNormalizeAndValidate(t *testing.T) {
	tests := []struct {
		slug string
		from string
		want string
	}{
		{"  US__East / 1  ", "", "us-east-1"},
		{"", " Frankfurt  Main ", "frankfurt-main"},
		{"--local--", "", "local"},
		{"---", "", ""},
		{"  ", "  ", ""},
		{"CAMEL_Case_Test", "", "camel-case-test"},
	}
	for _, tt := range tests {
		got := normalizeRegionSlug(tt.slug, tt.from)
		if got != tt.want {
			t.Errorf("normalizeRegionSlug(%q, %q) = %q, want %q", tt.slug, tt.from, got, tt.want)
		}
		if got != "" && !isValidRegionSlug(got) {
			t.Errorf("normalizeRegionSlug(%q, %q) = %q is not valid", tt.slug, tt.from, got)
		}
	}

	valid := []string{"us-east-1", "local", "r2d2", "a", "0", "multi-word-slug"}
	for _, slug := range valid {
		if !isValidRegionSlug(slug) {
			t.Errorf("isValidRegionSlug(%q) = false, want true", slug)
		}
	}
	invalid := []string{"US-EAST", "us--east", "us_east", "-local", "local-", "", "UPPER", "has spaces", "trailing-", "-leading"}
	for _, slug := range invalid {
		if isValidRegionSlug(slug) {
			t.Errorf("isValidRegionSlug(%q) = true, want false", slug)
		}
	}
}
