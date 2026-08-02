package redirects

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// PresetSkip records one thing a preset did not do, and why. Silently creating
// fewer rules than were asked for would leave the panel claiming "HTTPS enabled"
// while no rule exists, so every omission is named.
type PresetSkip struct {
	Preset PresetType `json:"preset"`
	Domain string     `json:"domain,omitempty"`
	Reason string     `json:"reason"`
}

// PresetApplyResult is what ApplyPresets actually did.
type PresetApplyResult struct {
	ApplicationID string       `json:"applicationId"`
	Created       []Redirect   `json:"created"`
	Skipped       []PresetSkip `json:"skipped"`
}

// ApplyPresets bulk-creates the common redirect patterns for an application from
// the hosts it is already bound to, and returns what was created alongside what
// was refused.
//
// It is idempotent: running "Enable HTTPS" twice creates nothing the second time
// and reports the existing rows as skipped rather than erroring. Creation stops
// on a store failure (the database is not answering) but never on a validation
// refusal, which is a per-rule answer about one host and should not abort the
// other hosts in the batch.
//
// The documented sketch returns a bare error; a preset that created three of
// five rules because two hosts are not bound is not an error and not a success
// either, so the outcome is reported explicitly.
func (s *Service) ApplyPresets(ctx context.Context, applicationID string, presets []PresetType) (PresetApplyResult, error) {
	result := PresetApplyResult{
		ApplicationID: strings.TrimSpace(applicationID),
		Created:       []Redirect{},
		Skipped:       []PresetSkip{},
	}
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return result, ErrEmptyAppID
	}
	if len(presets) == 0 {
		return result, invalidf("at least one preset is required")
	}

	// De-duplicate the request: asking for the same preset twice must not produce
	// two skip entries claiming "already exists" for work the first pass did.
	requested := make([]PresetType, 0, len(presets))
	seen := make(map[PresetType]struct{}, len(presets))
	for _, preset := range presets {
		normalized, err := ParsePresetType(string(preset))
		if err != nil {
			return result, err
		}
		if normalized == "" {
			return result, invalidf("a preset name must not be empty")
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}
		requested = append(requested, normalized)
	}

	domains, err := s.store.ListApplicationDomains(ctx, applicationID)
	if err != nil && !isMissingTableError(err) {
		return result, err
	}
	if isMissingTableError(err) {
		domains = nil
	}
	sort.Strings(domains)

	stored, err := s.store.ListByApplication(ctx, applicationID)
	if err != nil {
		if isMissingTableError(err) {
			// No table yet means no rules yet; the create below will surface the
			// real problem if the migration is still missing.
			stored = nil
		} else {
			return result, err
		}
	}

	for _, preset := range requested {
		plan, skip := s.planPreset(preset, domains)
		result.Skipped = append(result.Skipped, skip...)
		for _, candidate := range plan {
			candidate.ApplicationID = applicationID
			if err := validateCandidate(candidate, stored); err != nil {
				if conflicts(err) {
					result.Skipped = append(result.Skipped, PresetSkip{Preset: preset, Domain: candidate.SourceDomain, Reason: err.Error()})
					continue
				}
				return result, err
			}
			created, err := s.store.Create(ctx, candidate)
			if err != nil {
				if errors.Is(err, ErrDuplicateRedirect) {
					// Raced with another writer, or a rule the pre-check could not
					// see. Report it as skipped rather than failing the batch.
					result.Skipped = append(result.Skipped, PresetSkip{Preset: preset, Domain: candidate.SourceDomain, Reason: ErrDuplicateRedirect.Error()})
					continue
				}
				return result, err
			}
			result.Created = append(result.Created, created)
			// Keep the working set honest so a later preset in this same call is
			// checked against the rows that now exist.
			stored = append(stored, created)
		}
	}
	return result, nil
}

// planPreset turns one preset plus the application's hosts into the rules it
// implies. A plan may be empty with no skip reason only when the caller asked
// for a preset that needs hosts this application does not have — the reason is
// always reported, never implied.
func (s *Service) planPreset(preset PresetType, domains []string) ([]Redirect, []PresetSkip) {
	switch preset {
	case PresetHTTPToHTTPS:
		if len(domains) == 0 {
			return nil, []PresetSkip{{Preset: preset, Reason: "the application has no domains bound to it, so there is nothing to upgrade to HTTPS"}}
		}
		planned := make([]Redirect, 0, len(domains))
		for _, host := range domains {
			planned = append(planned, Redirect{
				SourceDomain: host,
				TargetDomain: host,
				SourcePath:   "/",
				TargetPath:   nil,
				StatusCode:   StatusMovedPermanently,
				PresetType:   PresetHTTPToHTTPS,
				Enabled:      true,
			})
		}
		return planned, nil

	case PresetWWWToApex:
		var planned []Redirect
		var skipped []PresetSkip
		found := false
		byHost := indexHosts(domains)
		for _, host := range domains {
			apex := ApexOf(host)
			if apex == host {
				continue
			}
			found = true
			if _, ok := byHost[apex]; !ok {
				skipped = append(skipped, PresetSkip{
					Preset: preset,
					Domain: host,
					Reason: fmt.Sprintf("%s is bound but its apex %s is not; bind %s first or the redirect will point at a host nothing serves", host, apex, apex),
				})
				continue
			}
			planned = append(planned, Redirect{
				SourceDomain: host,
				TargetDomain: apex,
				SourcePath:   "/",
				TargetPath:   nil,
				StatusCode:   StatusMovedPermanently,
				PresetType:   PresetWWWToApex,
				Enabled:      true,
			})
		}
		if !found && len(skipped) == 0 {
			skipped = append(skipped, PresetSkip{Preset: preset, Reason: "no www.* host is bound to this application"})
		}
		return planned, skipped

	case PresetApexToWWW:
		var planned []Redirect
		var skipped []PresetSkip
		found := false
		byHost := indexHosts(domains)
		for _, host := range domains {
			withWWW := WithWWWPrefix(host)
			if withWWW == "" {
				continue
			}
			found = true
			if _, ok := byHost[withWWW]; !ok {
				skipped = append(skipped, PresetSkip{
					Preset: preset,
					Domain: host,
					Reason: fmt.Sprintf("%s is bound but %s is not; bind %s first or the redirect will point at a host nothing serves", host, withWWW, withWWW),
				})
				continue
			}
			planned = append(planned, Redirect{
				SourceDomain: host,
				TargetDomain: withWWW,
				SourcePath:   "/",
				TargetPath:   nil,
				StatusCode:   StatusMovedPermanently,
				PresetType:   PresetApexToWWW,
				Enabled:      true,
			})
		}
		if !found && len(skipped) == 0 {
			skipped = append(skipped, PresetSkip{Preset: preset, Reason: "no apex host is bound to this application"})
		}
		return planned, skipped

	case PresetCustom:
		return nil, []PresetSkip{{
			Preset: preset,
			Reason: "custom is not a bulk preset; create the rule with POST /apps/{appId}/redirects",
		}}
	}

	// Unreachable through ParsePresetType, kept so a new PresetType cannot be
	// silently ignored by this switch.
	return nil, []PresetSkip{{Preset: preset, Reason: "this preset cannot be generated in bulk"}}
}

func indexHosts(domains []string) map[string]struct{} {
	index := make(map[string]struct{}, len(domains))
	for _, host := range domains {
		index[host] = struct{}{}
	}
	return index
}

// conflicts reports whether a validation failure is the "something equivalent is
// already there" family, which a preset treats as "nothing to do", as opposed to
// a genuine problem the caller must hear about.
func conflicts(err error) bool {
	return errors.Is(err, ErrDuplicateRedirect) || errors.Is(err, ErrCircularRedirect) || errors.Is(err, ErrSelfRedirect)
}
