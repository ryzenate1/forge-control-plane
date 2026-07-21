package crossnode

import (
	"fmt"
	"sort"
	"strings"

	"gamepanel/forge/internal/services/trafficmanager"
)

type RouteKey struct {
	Domain   string
	Path     string
	Protocol string
}

type RouteGroup struct {
	Key   RouteKey
	Rules []*trafficmanager.RoutingRule
	Ports []ServicePortRef
}

type ServicePortRef struct {
	ServerID   string
	NodeID     string
	TargetHost string
	TargetPort int
	ServiceID  string
	ServiceName string
}

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
				Ports: make([]ServicePortRef, 0),
			}
			groups[key] = grp
		}
		grp.Rules = append(grp.Rules, rule)
		grp.Ports = append(grp.Ports, ServicePortRef{
			ServerID:    rule.ServerID,
			TargetHost:  rule.TargetHost,
			TargetPort:  rule.TargetPort,
		})
	}
	return groups
}

func (g *RouteGroup) UniqueBackends() []BackendAddr {
	seen := make(map[string]bool)
	var backends []BackendAddr

	for _, rule := range g.Rules {
		host := rule.TargetHost
		if host == "" {
			host = "localhost"
		}
		addr := fmt.Sprintf("%s:%d", host, rule.TargetPort)
		if seen[addr] {
			continue
		}
		seen[addr] = true
		backends = append(backends, BackendAddr{
			Host:   host,
			Port:   rule.TargetPort,
			URL:    fmt.Sprintf("http://%s:%d", host, rule.TargetPort),
			Weight: rule.Weight,
		})
	}

	sort.Slice(backends, func(i, j int) bool {
		return backends[i].Host+":"+fmt.Sprint(backends[i].Port) <
			backends[j].Host+":"+fmt.Sprint(backends[j].Port)
	})
	return backends
}

func (g *RouteGroup) HasWebSocket() bool {
	for _, rule := range g.Rules {
		if rule.WebSocket {
			return true
		}
	}
	return false
}

func (g *RouteGroup) PolicyIDs() []string {
	seen := make(map[string]bool)
	var ids []string
	for _, rule := range g.Rules {
		pid := extractPolicyID(rule)
		if pid != "" && !seen[pid] {
			seen[pid] = true
			ids = append(ids, pid)
		}
	}
	return ids
}

func (g *RouteGroup) Strategy() string {
	for _, rule := range g.Rules {
		if rule.Strategy != "" && rule.Strategy != "round_robin" {
			return rule.Strategy
		}
	}
	return "round_robin"
}

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

func extractPolicyID(rule *trafficmanager.RoutingRule) string {
	return ""
}

type RouteGenerationRecord struct {
	RouteKey      RouteKey `json:"routeKey"`
	GroupID       string   `json:"groupId"`
	RuleIDs       []string `json:"ruleIds"`
	ServerIDs     []string `json:"serverIds"`
	BackendCount  int      `json:"backendCount"`
	HasWebSocket  bool     `json:"hasWebSocket"`
	Strategy      string   `json:"strategy"`
}

func BuildRouteGenerationRecords(groups map[RouteKey]*RouteGroup) []RouteGenerationRecord {
	var records []RouteGenerationRecord
	for key, grp := range groups {
		backends := grp.UniqueBackends()
		ruleIDs := make([]string, len(grp.Rules))
		for i, r := range grp.Rules {
			ruleIDs[i] = r.ID
		}
		records = append(records, RouteGenerationRecord{
			RouteKey:     key,
			GroupID:      groupID(key),
			RuleIDs:      ruleIDs,
			ServerIDs:    grp.ServiceIDs(),
			BackendCount: len(backends),
			HasWebSocket: grp.HasWebSocket(),
			Strategy:     grp.Strategy(),
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

func SimplifyDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.TrimPrefix(domain, "*.")
	return domain
}
