package store

import "testing"

func TestNormalizeRegionSlug(t *testing.T) {
	tests := []struct {
		name string
		slug string
		from string
		want string
	}{
		{name: "canonicalizes separators and case", slug: "  US__East / 1  ", want: "us-east-1"},
		{name: "falls back to name", from: " Frankfurt  Main ", want: "frankfurt-main"},
		{name: "drops leading and trailing punctuation", slug: "--local--", want: "local"},
		{name: "rejectable when no valid characters remain", slug: "---", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRegionSlug(tt.slug, tt.from)
			if got != tt.want {
				t.Fatalf("normalizeRegionSlug(%q, %q) = %q, want %q", tt.slug, tt.from, got, tt.want)
			}
			if got != "" && !isValidRegionSlug(got) {
				t.Fatalf("normalized slug %q is not valid", got)
			}
		})
	}
}

func TestIsValidRegionSlug(t *testing.T) {
	for _, slug := range []string{"us-east-1", "local", "r2d2"} {
		if !isValidRegionSlug(slug) {
			t.Errorf("isValidRegionSlug(%q) = false, want true", slug)
		}
	}
	for _, slug := range []string{"US-EAST", "us--east", "us_east", "-local", "local-", ""} {
		if isValidRegionSlug(slug) {
			t.Errorf("isValidRegionSlug(%q) = true, want false", slug)
		}
	}
}
