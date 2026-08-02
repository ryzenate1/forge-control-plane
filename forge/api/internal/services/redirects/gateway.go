package redirects

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// RedirectKind is what a rule actually changes. It is derived, never stored:
// the same row is a scheme upgrade, a host canonicalisation or a path move
// depending on which of the two endpoints differ.
type RedirectKind string

const (
	// KindSchemeUpgrade keeps host and path and moves http to https.
	KindSchemeUpgrade RedirectKind = "scheme-upgrade"
	// KindHost moves a request to a different host, path preserved or replaced.
	KindHost RedirectKind = "host"
	// KindPath moves a request to a different path on the same host.
	KindPath RedirectKind = "path"
)

// TraefikRegexRedirect mirrors the `redirectRegex` middleware Traefik's file
// provider accepts — the same three fields the traffic manager already knows how
// to serialise (trafficmanager.TraefikRedirectRegex).
type TraefikRegexRedirect struct {
	Regex       string `json:"regex"`
	Replacement string `json:"replacement"`
	Permanent   bool   `json:"permanent"`
}

// TraefikSchemeRedirect mirrors `redirectScheme`, which is the correct
// primitive for an HTTP-to-HTTPS upgrade: it rewrites only the scheme and port,
// so the path and query ride along untouched.
type TraefikSchemeRedirect struct {
	Scheme    string `json:"scheme"`
	Port      string `json:"port,omitempty"`
	Permanent bool   `json:"permanent"`
}

// TraefikRedirect is the middleware block for one rule. Exactly one of Regex
// and Scheme is populated; the other is nil so a caller can test for it instead
// of guessing from a zero string.
type TraefikRedirect struct {
	MiddlewareName string                 `json:"middlewareName"`
	RouterRule     string                 `json:"routerRule"`
	Regex          *TraefikRegexRedirect  `json:"redirectRegex,omitempty"`
	Scheme         *TraefikSchemeRedirect `json:"redirectScheme,omitempty"`
}

// CaddyRedirect is the route fragment a Caddy JSON config needs: a host match,
// an optional path match list, and the Location template to answer with. The
// traffic manager builds `static_response` handles from these fields, matching
// the shape of its existing HTTPS-redirect route.
type CaddyRedirect struct {
	RouteID string   `json:"routeId"`
	Hosts   []string `json:"hosts"`
	Paths   []string `json:"paths,omitempty"`
	// Location is a Caddy placeholder template, not a resolved URL:
	// "https://{http.request.host}{http.request.uri}" for a scheme upgrade,
	// "https://example.com{http.request.uri}" when the path is preserved, and a
	// literal path when the rule replaces it.
	//
	// Worth knowing the asymmetry: `{http.request.uri}` carries the query along,
	// so path-preserving rules keep it in both renderers. A rule with an explicit
	// target path answers with a literal Location and the query is dropped, which
	// is what Caddy's `redir` does without a named capture; the panel's default
	// (preserve the request path) avoids it.
	Location   string `json:"location"`
	StatusCode int    `json:"statusCode"`
}

// RedirectRule is the gateway-facing view of a stored rule: the same fact
// expressed once per front end the panel supports, plus the matching predicate
// the router needs. The traffic manager can inject these without knowing
// anything about how rules were authored.
type RedirectRule struct {
	ID            string       `json:"id"`
	ApplicationID string       `json:"applicationId"`
	Name          string       `json:"name"`
	Kind          RedirectKind `json:"kind"`
	SourceHost    string       `json:"sourceHost"`
	SourcePath    string       `json:"sourcePath"`
	// TargetURL is the concrete URL a request to SourcePath will be sent to,
	// shown for the UI and for a human reading the generated config. It is an
	// example, not the matcher: the templated forms live in Traefik/Caddy.
	TargetURL    string `json:"targetUrl"`
	StatusCode   int    `json:"statusCode"`
	Permanent    bool   `json:"permanent"`
	PreservePath bool   `json:"preservePath"`
	// MatchesAppDomain is false when the source host is not one of the domains
	// bound to this application, which means the rule is syntactically fine but
	// will never fire. Surfaced rather than hidden so a typo is visible.
	MatchesAppDomain bool            `json:"matchesAppDomain"`
	Traefik          TraefikRedirect `json:"traefik"`
	Caddy            CaddyRedirect   `json:"caddy"`
}

// GenerateGatewayConfig renders every enabled rule for one application into the
// structured form the traffic manager injects into Traefik middlewares or Caddy
// routes. Disabled rules are excluded (the operator turned them off, so the
// gateway must not enforce them) and the result is ordered by name so two calls
// with the same data produce byte-identical output — a diff of the generated
// config is only useful if unrelated reorders do not appear in it.
func (s *Service) GenerateGatewayConfig(ctx context.Context, applicationID string) ([]RedirectRule, error) {
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return nil, ErrEmptyAppID
	}
	rules, err := s.List(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	// The domain list is advisory: it decides MatchesAppDomain and nothing else.
	// A store that cannot answer it must not block generating config for rules
	// the operator explicitly wrote, so the failure degrades to "unknown host
	// set" instead of an error.
	domains, domainErr := s.store.ListApplicationDomains(ctx, applicationID)
	if domainErr != nil && !isMissingTableError(domainErr) {
		return nil, domainErr
	}
	known := make(map[string]struct{}, len(domains))
	for _, host := range domains {
		known[host] = struct{}{}
	}

	generated := make([]RedirectRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		generated = append(generated, renderRule(rule, known))
	}
	sort.SliceStable(generated, func(i, j int) bool { return generated[i].Name < generated[j].Name })
	return generated, nil
}

// renderRule turns one stored rule into both gateway dialects.
func renderRule(rule Redirect, knownHosts map[string]struct{}) RedirectRule {
	kind := classify(rule)
	permanent := IsPermanent(rule.StatusCode)
	name := ruleName(rule)
	preserve := rule.TargetPath == nil

	targetPath := ""
	if !preserve {
		targetPath = *rule.TargetPath
	}
	// A nil target path means "keep the request path"; an explicit "" means
	// "send everything to the target root". Both are meaningful, and conflating
	// them is the classic way a canonicalisation rule starts swallowing deep
	// links. TargetURL is the human-readable answer, not the matcher.
	targetURL := "https://" + rule.TargetDomain
	switch {
	case kind == KindSchemeUpgrade:
		// Same host, same path: only the scheme changes.
	case !preserve && targetPath == "":
		targetURL += "/"
	case !preserve:
		targetURL += targetPath
	case rule.SourcePath != "/" && rule.SourcePath != "":
		targetURL += rule.SourcePath
	}

	out := RedirectRule{
		ID:               rule.ID,
		ApplicationID:    rule.ApplicationID,
		Name:             name,
		Kind:             kind,
		SourceHost:       rule.SourceDomain,
		SourcePath:       rule.SourcePath,
		TargetURL:        targetURL,
		StatusCode:       rule.StatusCode,
		Permanent:        permanent,
		PreservePath:     preserve,
		MatchesAppDomain: matchesKnownHost(knownHosts, rule.SourceDomain),
	}

	// --- Traefik -----------------------------------------------------------
	out.Traefik = TraefikRedirect{
		MiddlewareName: name,
		RouterRule:     fmt.Sprintf("Host(`%s`)", rule.SourceDomain),
	}
	if kind == KindSchemeUpgrade {
		out.Traefik.Scheme = &TraefikSchemeRedirect{
			Scheme:    "https",
			Port:      "443",
			Permanent: permanent,
		}
	} else {
		regex, replacement := traefikRedirectRegex(rule, preserve, targetPath)
		out.Traefik.Regex = &TraefikRegexRedirect{
			Regex:       regex,
			Replacement: replacement,
			Permanent:   permanent,
		}
	}

	// --- Caddy -------------------------------------------------------------
	out.Caddy = CaddyRedirect{
		RouteID:    name,
		Hosts:      []string{rule.SourceDomain},
		Paths:      caddyPaths(rule.SourcePath),
		StatusCode: rule.StatusCode,
	}
	switch {
	case kind == KindSchemeUpgrade:
		out.Caddy.Location = schemeUpgradeLocationTmpl
	case preserve:
		out.Caddy.Location = "https://" + rule.TargetDomain + "{http.request.uri}"
	case targetPath == "":
		out.Caddy.Location = "https://" + rule.TargetDomain + "/"
	default:
		out.Caddy.Location = "https://" + rule.TargetDomain + targetPath
	}
	return out
}

// classify decides what a rule changes. Scheme upgrades are recognised by their
// preset label *and* identical endpoints, so a hand-written row that happens to
// have the same host on both sides cannot quietly become an HTTPS rule.
func classify(rule Redirect) RedirectKind {
	if rule.SchemeUpgrade() {
		return KindSchemeUpgrade
	}
	if rule.SourceDomain != rule.TargetDomain {
		return KindHost
	}
	return KindPath
}

func matchesKnownHost(known map[string]struct{}, host string) bool {
	if len(known) == 0 {
		return false
	}
	_, ok := known[host]
	// A host is only "served by this application" when it is itself listed.
	// Assuming the apex implies www (or the reverse) would report a rule as live
	// when the certificate probably does not cover it.
	return ok
}

// ruleName builds a stable, gateway-safe identifier: both Traefik middleware
// names and Caddy route ids must survive a config reload unchanged, so the name
// is derived from the two ids and never from the (editable) domains.
func ruleName(rule Redirect) string {
	return fmt.Sprintf("%s-%s-%s", defaultGatewayNamePrefix, shortID(rule.ApplicationID), shortID(rule.ID))
}

func shortID(id string) string {
	compact := strings.ReplaceAll(id, "-", "")
	if len(compact) <= 8 {
		if compact == "" {
			return "00000000"
		}
		return compact
	}
	return compact[:8]
}

// traefikRedirectRegex builds the URL pattern `redirectRegex` matches against
// and the replacement it answers with. redirectRegex works on the full request
// URL, so the host is escaped, an optional port is tolerated (a request arriving
// on :8080 should still be canonicalised) and the tail is captured in $1.
//
// A source path is treated as a prefix: "/old" matches "/old" and "/old/anything",
// and never "/older", because the captured remainder is required to start with a
// slash.
func traefikRedirectRegex(rule Redirect, preserve bool, targetPath string) (string, string) {
	host := regexp.QuoteMeta(rule.SourceDomain)
	base := "https://" + rule.TargetDomain

	if rule.SourcePath == "" || rule.SourcePath == "/" {
		regex := fmt.Sprintf(`^https?://%s(?::\d+)?(/.*)?$`, host)
		if !preserve && targetPath == "" {
			// Everything lands on the target root: the tail is deliberately dropped.
			return regex, base
		}
		if preserve {
			return regex, base + "$1"
		}
		return regex, base + targetPath + "$1"
	}

	prefix := strings.TrimSuffix(rule.SourcePath, "/*")
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" {
		prefix = "/"
	}
	regex := fmt.Sprintf(`^https?://%s(?::\d+)?%s(/.*)?$`, host, regexp.QuoteMeta(prefix))
	if !preserve && targetPath == "" {
		return regex, base
	}
	keptPrefix := prefix
	if !preserve {
		keptPrefix = targetPath
	}
	return regex, base + keptPrefix + "$1"
}

// caddyPaths turns a source path into the globs Caddy's path matcher accepts.
// "/" matches everything, so it is emitted as no path matcher at all; a concrete
// path is emitted both exactly and as its subtree, because an operator who
// writes "/old" means "/old and everything under it".
func caddyPaths(sourcePath string) []string {
	if sourcePath == "" || sourcePath == "/" {
		return nil
	}
	if strings.HasSuffix(sourcePath, "/*") {
		return []string{sourcePath, strings.TrimSuffix(sourcePath, "*")}
	}
	return []string{sourcePath, sourcePath + "/*"}
}

// isMissingTableError recognises "the migration has not run yet" so a freshly
// upgraded panel can still serve requests that do not touch the table, the same
// way the traffic manager treats a missing traffic_rules table.
func isMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "undefined table")
}
