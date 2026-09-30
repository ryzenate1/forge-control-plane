// Package redirects stores per-application HTTP redirect rules and renders them
// into the shape the gateway layer consumes.
//
// It is the Forge counterpart to Dokploy's `redirects` router plus its
// `utils/traefik/redirect.ts` middleware writer: Dokploy keeps one row per
// (application, regex/replacement) pair and pushes a `redirectRegex` middleware
// into the Traefik file for that application. Forge has the same two jobs —
// remember the intent, then express it in whatever the front-end speaks — so
// this package owns the intent and emits a structured RedirectRule that the
// traffic manager can turn into a Traefik middleware or a Caddy route.
//
// The four things operators actually ask for are encoded as presets so they are
// one click instead of a regex:
//
//   - http-to-https — same host, scheme upgraded (Traefik redirectScheme,
//     Caddy Location `https://{host}{uri}`);
//   - www-apex      — www.example.com redirects to example.com;
//   - apex-www      — the inverse, for sites that canonicalise on www;
//   - custom        — an explicit (host, path) → (host, path) rule, which is
//     also what the path-based 301/302 case uses.
//
// Layering follows the panel convention: handlers -> Service -> Store, with the
// Store defined and implemented here over the shared pgx pool so the feature
// stays entirely additive (nothing is added to internal/store).
package redirects

import (
	"errors"
	"fmt"
	"net"
	"path"
	"strings"
	"time"
)

// PresetType labels how a rule was produced. It is stored (nullable) purely so
// the UI can group hand-written rules from generated ones and so ApplyPresets
// can recognise its own output as already present.
type PresetType string

const (
	// PresetHTTPToHTTPS upgrades the scheme for one host: source and target
	// domain are identical, which is the only case where that is allowed.
	PresetHTTPToHTTPS PresetType = "http-to-https"
	// PresetWWWToApex canonicalises www.example.com to example.com.
	PresetWWWToApex PresetType = "www-apex"
	// PresetApexToWWW canonicalises example.com to www.example.com.
	PresetApexToWWW PresetType = "apex-www"
	// PresetCustom marks a rule an operator wrote by hand.
	PresetCustom PresetType = "custom"
)

func (p PresetType) Valid() bool {
	switch p {
	case PresetHTTPToHTTPS, PresetWWWToApex, PresetApexToWWW, PresetCustom:
		return true
	}
	return false
}

// ParsePresetType normalises user input. An empty string means "not a preset"
// rather than "invalid preset", so callers receive ("", nil) and must treat an
// absent label as custom/unset.
func ParsePresetType(raw string) (PresetType, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", nil
	}
	preset := PresetType(trimmed)
	if !preset.Valid() {
		return "", invalidf("unknown preset %q: expected one of http-to-https, www-apex, apex-www, custom", raw)
	}
	return preset, nil
}

// Status codes this feature emits. Everything else is rejected: 300/305 are
// legacy or proxy-specific, and 3xx that a browser will not follow automatically
// would silently break the redirect the operator asked for.
const (
	StatusMovedPermanently    = 301
	StatusFound               = 302
	StatusTemporaryRedirect   = 307
	StatusPermanentRedirect   = 308
	DefaultStatusCode         = StatusMovedPermanently
	MaxRulesPerApplication    = 64
	wwwPrefix                 = "www."
	defaultGatewayNamePrefix  = "forge-redirect"
	schemeUpgradeLocationTmpl = "https://{http.request.host}{http.request.uri}"
)

// ValidStatusCode reports whether a status code is one the gateway can be asked
// to emit for a redirect.
func ValidStatusCode(code int) bool {
	switch code {
	case StatusMovedPermanently, StatusFound, StatusTemporaryRedirect, StatusPermanentRedirect:
		return true
	}
	return false
}

// IsPermanent maps a status code onto the single boolean both Traefik's
// redirect middleware and Caddy's redirection accept. RFC 9110 reserves 301 and
// 308 as cacheable-forever, 302 and 307 as temporary; treating anything else as
// "permanent" would let a mis-typed code poison browser caches.
func IsPermanent(statusCode int) bool {
	return statusCode == StatusMovedPermanently || statusCode == StatusPermanentRedirect
}

// Sentinel errors surfaced to the HTTP layer, which translates them into statuses.
var (
	// ErrApplicationNotFound is returned when the application row does not exist.
	ErrApplicationNotFound = errors.New("application not found")
	// ErrRedirectNotFound is returned when the (application, redirect) pair is
	// absent. Update and Delete treat it as an error, never as a silent success.
	ErrRedirectNotFound = errors.New("redirect rule not found")
	// ErrEmptyAppID guards every entry point against a blank application id.
	ErrEmptyAppID = errors.New("application id is required")
	// ErrDuplicateRedirect means an identical (source domain, source path,
	// target domain) rule is already stored — the database enforces this too.
	ErrDuplicateRedirect = errors.New("an identical redirect rule already exists for this application")
	// ErrCircularRedirect refuses A→B while B→A is enabled, which would make
	// browsers bounce between the two hosts until they give up.
	ErrCircularRedirect = errors.New("the reverse redirect is already enabled; enabling both would loop")
	// ErrSelfRedirect refuses a rule whose source and target resolve to the same
	// URL: the gateway would answer a redirect with the same request forever.
	ErrSelfRedirect = errors.New("a redirect must change the host, the path, or the scheme")
	// ErrTooManyRules bounds one application's rule set.
	ErrTooManyRules = errors.New("this application already has the maximum number of redirect rules")
)

// ValidationError marks an error the caller can fix by sending different values,
// as opposed to a failure inside the platform. Without it the handler could only
// guess the status from message text.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalidf(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// Redirect is one stored rule: "when a request arrives for sourceDomain and
// matches sourcePath, send it to targetDomain/targetPath with this status".
//
// TargetPath is nullable and the null is meaningful: nil means "preserve the
// request path" (the common case for a host canonicalisation), while "" would
// mean "redirect to the target root". SourcePath "/" means "every path on this
// host".
type Redirect struct {
	ID            string     `json:"id"`
	ApplicationID string     `json:"applicationId"`
	SourceDomain  string     `json:"sourceDomain"`
	TargetDomain  string     `json:"targetDomain"`
	SourcePath    string     `json:"sourcePath"`
	TargetPath    *string    `json:"targetPath,omitempty"`
	StatusCode    int        `json:"statusCode"`
	PresetType    PresetType `json:"presetType,omitempty"`
	Enabled       bool       `json:"enabled"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// CreateRequest carries the fields for a new rule. StatusCode and Enabled are
// pointers so "omit" and "zero" stay distinguishable: omitting StatusCode means
// 301, sending 0 means "I sent nonsense" and must be rejected, not defaulted.
type CreateRequest struct {
	SourceDomain string     `json:"sourceDomain"`
	TargetDomain string     `json:"targetDomain"`
	SourcePath   *string    `json:"sourcePath"`
	TargetPath   *string    `json:"targetPath"`
	StatusCode   *int       `json:"statusCode"`
	PresetType   PresetType `json:"presetType"`
	Enabled      *bool      `json:"enabled"`
}

// UpdateRequest carries patchable fields; nil means "leave unchanged".
//
// Two extra knobs exist because a plain pointer cannot express "unset this":
// ClearTargetPath sets the target back to "preserve the request path", and
// PresetType is only honoured when non-nil (an empty string is a caller error,
// not a request to blank the column).
type UpdateRequest struct {
	SourceDomain    *string `json:"sourceDomain"`
	TargetDomain    *string `json:"targetDomain"`
	SourcePath      *string `json:"sourcePath"`
	TargetPath      *string `json:"targetPath"`
	ClearTargetPath bool    `json:"clearTargetPath"`
	StatusCode      *int    `json:"statusCode"`
	Enabled         *bool   `json:"enabled"`
	PresetType      *string `json:"presetType"`
}

// ---------------------------------------------------------------------------
// Normalisation and validation
// ---------------------------------------------------------------------------

// NormalizeDomain turns anything an operator typed into a bare lowercase DNS
// hostname: "https://WWW.Example.com:8443/old" becomes "www.example.com". It
// rejects addresses that are not hostnames (IPv4/IPv6 literals, wildcards,
// credentials, stray spaces) because a gateway host matcher on those would
// either never fire or fire for every request.
func NormalizeDomain(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", invalidf("a domain is required")
	}
	// Accept a URL the operator pasted from a browser: strip scheme, then any
	// path/query/fragment, then an optional port.
	if idx := strings.Index(value, "://"); idx >= 0 {
		value = value[idx+3:]
	}
	if idx := strings.IndexAny(value, "/?#"); idx >= 0 {
		value = value[:idx]
	}
	if idx := strings.LastIndex(value, "@"); idx >= 0 {
		return "", invalidf("a redirect domain must not contain credentials")
	}
	if idx := strings.LastIndex(value, ":"); idx >= 0 {
		// Only a port if what follows is all digits; otherwise the colon is part
		// of something invalid (an IPv6 literal or a stray separator).
		tail := value[idx+1:]
		if tail != "" && strings.TrimRight(tail, "0123456789") == "" {
			value = value[:idx]
		}
	}
	value = strings.ToLower(strings.Trim(value, "."))
	if value == "" {
		return "", invalidf("a domain is required")
	}
	if strings.ContainsAny(value, " \t") {
		return "", invalidf("%q is not a valid domain: it contains whitespace", raw)
	}
	if strings.HasPrefix(value, "*") {
		return "", invalidf("wildcard domains cannot be redirect sources or targets; name the concrete host")
	}
	if isValidDomainName(value) {
		return value, nil
	}
	// An IPv4 literal is a legitimate redirect host even though it is not a DNS
	// name; the gateway matches on the Host header, so it has to be exact.
	if ip := net.ParseIP(value); ip != nil && !strings.Contains(value, ":") {
		return value, nil
	}
	if strings.Contains(value, ":") {
		return "", invalidf("IPv6 redirect hosts are not supported; use a domain name")
	}
	return "", invalidf("%q is not a valid domain", raw)
}

// isValidDomainName reports whether value is a well-formed DNS hostname:
// at most 253 chars, dot-separated labels of 1-63 [a-z0-9-] chars that
// neither start nor end with a hyphen. (The std net package offers no
// domain-name validator, so the check lives here next to its only caller.)
// Callers pass a lowercased value, so only lowercase is accepted.
func isValidDomainName(value string) bool {
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		for i := 0; i < len(label); i++ {
			ch := label[i]
			if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-' {
				continue
			}
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
	}
	return true
}

// NormalizePath turns a typed path into a clean absolute one. An empty input
// becomes "/" — the default for a source path, and the caller that wants
// "preserve the request path" for a target should pass nil, not "".
func NormalizePath(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "/", nil
	}
	if !strings.HasPrefix(value, "/") {
		return "", invalidf("path %q must start with /", raw)
	}
	if strings.ContainsAny(value, "?#") {
		return "", invalidf("path %q must not contain a query string or fragment; the gateway preserves those from the request", raw)
	}
	cleaned := path.Clean(value)
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "/../") {
		return "", invalidf("path %q must not traverse upwards", raw)
	}
	return cleaned, nil
}

// NormalizeTargetPath is NormalizePath for the optional target path. nil and
// ""-after-trim are kept distinct: nil means "preserve the request path", so it
// returns nil rather than "/".
func NormalizeTargetPath(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	if strings.TrimSpace(*raw) == "" {
		empty := ""
		return &empty, nil
	}
	cleaned, err := NormalizePath(*raw)
	if err != nil {
		return nil, err
	}
	return &cleaned, nil
}

// ApexOf returns the zone apex for a host ("www.example.com" -> "example.com").
// Only the leading "www." label is removed: a preset must not guess that
// "shop.example.com" and "example.com" are the same site.
func ApexOf(host string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(host)), wwwPrefix)
}

// WithWWWPrefix returns the www form of a host, or "" if it already carries the
// label. "a.b.example.com" is deliberately rejected: prefixing an arbitrary
// sub-sub-domain is not what the apex-www preset means.
func WithWWWPrefix(host string) string {
	normalized := strings.ToLower(strings.TrimSpace(host))
	if normalized == "" || strings.HasPrefix(normalized, wwwPrefix) {
		return ""
	}
	if strings.Count(normalized, ".") < 1 {
		return ""
	}
	return wwwPrefix + normalized
}

// PreservedPath reports whether a rule keeps the incoming request path.
func (r Redirect) PreservedPath() bool { return r.TargetPath == nil }

// SchemeUpgrade reports whether a rule only changes the scheme. Such rules
// legitimately have identical source and target domains — every other pair does
// not, and would be a self-redirect.
func (r Redirect) SchemeUpgrade() bool {
	return r.PresetType == PresetHTTPToHTTPS && r.SourceDomain == r.TargetDomain && r.SourcePath == "/" && (r.TargetPath == nil || *r.TargetPath == "/")
}

// validateCandidate checks a fully-resolved rule against the rules already
// stored for the same application. It is the single place where the "would this
// redirect do anything sensible?" question is answered, so Create, Update and
// the preset path all share it.
func validateCandidate(candidate Redirect, existing []Redirect) error {
	if strings.TrimSpace(candidate.ApplicationID) == "" {
		return ErrEmptyAppID
	}
	if candidate.StatusCode != 0 && !ValidStatusCode(candidate.StatusCode) {
		return invalidf("status code %d is not supported: choose 301, 302, 307 or 308", candidate.StatusCode)
	}
	if candidate.SourceDomain == "" || candidate.TargetDomain == "" {
		return invalidf("both a source domain and a target domain are required")
	}
	if !candidate.SchemeUpgrade() && candidate.SourceDomain == candidate.TargetDomain {
		// Same host. Only a path change can make it meaningful.
		target := candidate.TargetPath
		if target == nil || *target == candidate.SourcePath {
			return ErrSelfRedirect
		}
	}
	if len(existing) >= MaxRulesPerApplication {
		return ErrTooManyRules
	}
	for _, rule := range existing {
		if rule.ID != "" && rule.ID == candidate.ID {
			continue
		}
		if sameRuleShape(rule, candidate) {
			return ErrDuplicateRedirect
		}
		if rule.Enabled && oppositeOf(rule, candidate) {
			return ErrCircularRedirect
		}
	}
	return nil
}

// sameRuleShape compares the identity of two rules as the unique index does:
// source host + source path + target host. The target path and status code are
// deliberately excluded — "the same pair of endpoints twice with different
// intent" is the mistake worth blocking, and allowing it would leave one of the
// two rules unreachable in the gateway.
func sameRuleShape(a, b Redirect) bool {
	return a.SourceDomain == b.SourceDomain && a.SourcePath == b.SourcePath && a.TargetDomain == b.TargetDomain
}

// oppositeOf reports whether two rules point at each other's endpoints, which
// is a redirect loop regardless of which host the request arrived at.
func oppositeOf(a, b Redirect) bool {
	if a.SchemeUpgrade() || b.SchemeUpgrade() {
		// Scheme upgrades are idempotent (the target is https), so pairing one
		// with a same-host rule cannot ping-pong.
		return false
	}
	return a.SourceDomain == b.TargetDomain && a.TargetDomain == b.SourceDomain && a.SourcePath == b.SourcePath
}
