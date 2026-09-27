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

// UniqueBackends collapses the group's rules into one backend per host:port.
//
// Several rules may name the same backend with different weights; the weights
// are summed instead of resolved by "whichever rule came first", because rules
// reach this package through a Go map and the first one visited is not stable
// across syncs. A non-positive weight counts as 1 so an unconfigured rule keeps
// the smallest schedulable share instead of a zero that would make it dead.
func (g *RouteGroup) UniqueBackends() []BackendAddr {
	index := make(map[string]int, len(g.Rules))
	var backends []BackendAddr

	for _, rule := range g.Rules {
		host := rule.TargetHost
		if host == "" {
			host = "localhost"
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

// Strategy returns the group's effective strategy: the first non-default
// strategy in rule-ID order, or round_robin when no rule asks for one. If the
// group mixes more than one non-default strategy the earliest rule wins here
// and StrategyConflict reports the losers, so callers never see a silent
// precedence.
func (g *RouteGroup) Strategy() string {
	for _, rule := range g.Rules {
		if rule.Strategy != "" && rule.Strategy != defaultStrategy {
			return rule.Strategy
		}
	}
	return defaultStrategy
}

// StrategyConflict reports whether the group's rules disagree on strategy.
// The string is the strategies that lost to Strategy()'s choice, joined for
// display; the bool is true only when such a conflict exists.
func (g *RouteGroup) StrategyConflict() (string, bool) {
	seen := make(map[string]bool, len(g.Rules))
	for _, rule := range g.Rules {
		if rule.Strategy != "" && rule.Strategy != defaultStrategy {
			seen[rule.Strategy] = true
		}
	}
	winner := g.Strategy()
	var losers []string
	for strategy := range seen {
		if strategy != winner {
			losers = append(losers, strategy)
		}
	}
	if len(losers) == 0 {
		return "", false
	}
	sort.Strings(losers)
	return strings.Join(losers, ", "), true
}

// ServiceIDs returns the distinct server IDs referenced by the group's rules,
// in first-by-rule-ID order.
func (g *RouteGroup) ServiceIDs() []string {
	seen := make(map[string]bool)
	var ids []string
	for _, rule := range g.Rules {
		if rule.ServerID != "" && !seen[rule.ServerID] {
			seen[rule.ServerID] = true
			ids = append(ids, rule.ServerID)
		}
	}
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
		for i, r := range grp.Rules {
			ruleIDs[i] = r.ID
		}
		conflict, _ := grp.StrategyConflict()
		records = append(records, RouteGenerationRecord{
			RouteKey:         key,
			GroupID:          groupID(key),
			RuleIDs:          ruleIDs,
			ServerIDs:        grp.ServiceIDs(),
			BackendCount:     len(backends),
			HasWebSocket:     grp.HasWebSocket(),
			Strategy:         grp.Strategy(),
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
