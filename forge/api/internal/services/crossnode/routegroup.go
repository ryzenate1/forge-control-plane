package crossnode

import (
	"fmt"
	"sort"
	"strings"

	"gamepanel/forge/internal/services/trafficmanager"
)

// defaultStrategy is what a route does when no rule in the group asks for a
// different load-balancing strategy.
const defaultStrategy = "round_robin"

// RouteKey identifies one ingress route, i.e. the match target every rule in
// the group shares. Domain is used verbatim, so a wildcard host
// ("*.example.com") stays distinct from an exact host ("example.com"): Caddy
// and Traefik match them differently and they must never merge into one group.
type RouteKey struct {
	Domain   string
	Path     string
	Protocol string
}

// RouteGroup is the set of enabled routing rules that resolve to one RouteKey.
type RouteGroup struct {
	Key   RouteKey
	Rules []*trafficmanager.RoutingRule
}

// GroupRulesByRoute buckets enabled rules by (domain, path, protocol). An
// empty path means the route "/" and an empty protocol means plain http, so
// rules that describe the same route land together. Each group's Rules are
// sorted by rule ID before returning, so everything downstream is deterministic
// even if a caller hands us raw map-iteration order.
func GroupRulesByRoute(rules []*trafficmanager.RoutingRule) map[RouteKey]*RouteGroup {
	groups := make(map[RouteKey]*RouteGroup)

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		key := RouteKey{
			Domain:   rule.Domain,
			Path:     rule.Path,
			Protocol: rule.Protocol,
		}
		if key.Path == "" {
			key.Path = "/"
		}
		if key.Protocol == "" {
			key.Protocol = "http"
		}

		grp, ok := groups[key]
		if !ok {
			grp = &RouteGroup{
				Key:   key,
				Rules: make([]*trafficmanager.RoutingRule, 0),
			}
			groups[key] = grp
		}
		grp.Rules = append(grp.Rules, rule)
	}

	for _, grp := range groups {
		sort.Slice(grp.Rules, func(i, j int) bool {
			return grp.Rules[i].ID < grp.Rules[j].ID
		})
	}
	return groups
}

// UniqueBackends collapses the group's rules into one backend per host:port and
// sums weights, so the (host, port) sorted result is identical for the same rule
// set in any order. A non-positive weight counts as 1. A rule with no TargetHost
// contributes no backend: an unknown address must surface as a missing backend,
// never as a routable-looking one.
func (g *RouteGroup) UniqueBackends() []BackendAddr {
	index := make(map[string]int, len(g.Rules))
	var backends []BackendAddr

	for _, rule := range g.Rules {
		host := rule.TargetHost
		if host == "" {
			continue
		}
		weight := rule.Weight
		if weight <= 0 {
			weight = 1
		}
		addr := fmt.Sprintf("%s:%d", host, rule.TargetPort)
		if i, ok := index[addr]; ok {
			backends[i].Weight += weight
			continue
		}
		index[addr] = len(backends)
		backends = append(backends, BackendAddr{
			Host:   host,
			Port:   rule.TargetPort,
			URL:    fmt.Sprintf("http://%s:%d", host, rule.TargetPort),
			Weight: weight,
		})
	}

	sort.Slice(backends, func(i, j int) bool {
		if backends[i].Host != backends[j].Host {
			return backends[i].Host < backends[j].Host
		}
		return backends[i].Port < backends[j].Port
	})
	return backends
}

// HasWebSocket reports whether any rule in the group needs a WebSocket upgrade.
func (g *RouteGroup) HasWebSocket() bool {
	for _, rule := range g.Rules {
		if rule.WebSocket {
			return true
		}
	}
	return false
}

// Strategy returns the group's effective strategy: the lexicographically
// smallest non-default strategy any rule asks for, or round_robin when none
// does. Sorting the candidates, rather than taking the first rule's, is what
// keeps the answer stable when a group's rules disagree.
func (g *RouteGroup) Strategy() string {
	var candidates []string
	for _, rule := range g.Rules {
		if rule.Strategy != "" && rule.Strategy != defaultStrategy {
			candidates = append(candidates, rule.Strategy)
		}
	}
	if len(candidates) == 0 {
		return defaultStrategy
	}
	sort.Strings(candidates)
	return candidates[0]
}

// ServiceIDs returns the distinct server IDs referenced by the group's rules,
// sorted so the displayed list does not shuffle between syncs.
func (g *RouteGroup) ServiceIDs() []string {
	seen := make(map[string]bool)
	var ids []string
	for _, rule := range g.Rules {
		if rule.ServerID != "" && !seen[rule.ServerID] {
			seen[rule.ServerID] = true
			ids = append(ids, rule.ServerID)
		}
	}
	sort.Strings(ids)
	return ids
}

type BackendAddr struct {
	Host   string
	Port   int
	URL    string
	Weight int
}

type RouteGenerationRecord struct {
	RouteKey         RouteKey `json:"routeKey"`
	GroupID          string   `json:"groupId"`
	RuleIDs          []string `json:"ruleIds"`
	ServerIDs        []string `json:"serverIds"`
	BackendCount     int      `json:"backendCount"`
	HasWebSocket     bool     `json:"hasWebSocket"`
	Strategy         string   `json:"strategy"`
	StrategyConflict string   `json:"strategyConflict,omitempty"`
}

func BuildRouteGenerationRecords(groups map[RouteKey]*RouteGroup) []RouteGenerationRecord {
	var records []RouteGenerationRecord
	for key, grp := range groups {
		backends := grp.UniqueBackends()
		ruleIDs := make([]string, len(grp.Rules))
		alternatives := make(map[string]bool)
		for i, r := range grp.Rules {
			ruleIDs[i] = r.ID
			if r.Strategy != "" && r.Strategy != defaultStrategy {
				alternatives[r.Strategy] = true
			}
		}
		sort.Strings(ruleIDs)

		strategy := grp.Strategy()
		delete(alternatives, strategy)
		var conflict string
		if len(alternatives) > 0 {
			losers := make([]string, 0, len(alternatives))
			for s := range alternatives {
				losers = append(losers, s)
			}
			sort.Strings(losers)
			conflict = strings.Join(losers, ", ")
		}

		records = append(records, RouteGenerationRecord{
			RouteKey:         key,
			GroupID:          groupID(key),
			RuleIDs:          ruleIDs,
			ServerIDs:        grp.ServiceIDs(),
			BackendCount:     len(backends),
			HasWebSocket:     grp.HasWebSocket(),
			Strategy:         strategy,
			StrategyConflict: conflict,
		})
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].GroupID < records[j].GroupID
	})
	return records
}

func groupID(key RouteKey) string {
	return fmt.Sprintf("%s/%s/%s", key.Domain, key.Path, key.Protocol)
}
