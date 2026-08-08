package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	firewallFilterChain = "FORGE-BEACON"
	firewallNATChain    = "FORGE-BEACON-FWD"
)

type FirewallRule struct {
	ID          string `json:"id"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	SourceIP    string `json:"sourceIp"`
	Action      string `json:"action"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
}

type PortForward struct {
	ID          string `json:"id"`
	FromPort    int    `json:"fromPort"`
	ToPort      int    `json:"toPort"`
	ToIP        string `json:"toIp"`
	Protocol    string `json:"protocol"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
}

type firewallState struct {
	Enabled  bool                    `json:"enabled"`
	Rules    map[string]FirewallRule `json:"rules"`
	Forwards map[string]PortForward  `json:"forwards"`
}

type firewallData struct {
	mu        sync.RWMutex
	initOnce  sync.Once
	initErr   error
	statePath string
	state     firewallState
}

var firewallExec = runFirewallCommand
var firewallRestore = runFirewallRestore

func (d *firewallData) path() string {
	if d.statePath != "" {
		return d.statePath
	}
	if configured := strings.TrimSpace(os.Getenv("BEACON_FIREWALL_STATE_PATH")); configured != "" {
		return configured
	}
	dataDir := strings.TrimSpace(os.Getenv("DAEMON_DATA_DIR"))
	if dataDir == "" {
		dataDir = "/srv/game-panel/servers"
	}
	return filepath.Join(dataDir, ".beacon", "firewall.json")
}

func runFirewallCommand(ctx context.Context, name string, args ...string) error {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(commandCtx, name, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, message)
		}
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func runFirewallRestore(ctx context.Context, rules string) error {
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(commandCtx, "iptables-restore", "--wait", "10", "--noflush")
	cmd.Stdin = strings.NewReader(rules)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apply firewall transaction: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func validateFirewallProtocol(proto string) (string, error) {
	switch lower := strings.ToLower(strings.TrimSpace(proto)); lower {
	case "tcp", "udp":
		return lower, nil
	case "tcp/udp", "udp/tcp":
		return "", errors.New("combined protocol tcp/udp is not supported; use separate rules")
	default:
		return "", fmt.Errorf("unsupported protocol %q", proto)
	}
}

func validateFirewallAction(action string) (string, error) {
	switch lower := strings.ToLower(strings.TrimSpace(action)); lower {
	case "", "allow", "accept", "open":
		return "allow", nil
	default:
		return "", fmt.Errorf("unsupported action %q: only allow/accept is supported", action)
	}
}

func validateFirewallSource(source string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return errors.New("source IP or CIDR is required; unrestricted rules are not allowed")
	}
	if ip := net.ParseIP(source); ip != nil {
		if !safeFirewallSourceIP(ip) {
			return fmt.Errorf("source IP %q is not a permitted public unicast address", source)
		}
		return nil
	}
	if ip, network, err := net.ParseCIDR(source); err == nil {
		ones, _ := network.Mask.Size()
		if ones == 0 || !ip.Equal(network.IP) || !safeFirewallSourceIP(ip) {
			return fmt.Errorf("source CIDR %q is unrestricted, non-canonical, or non-public", source)
		}
		return nil
	}
	return fmt.Errorf("invalid source IP or CIDR %q", source)
}

func canonicalFirewallSource(source string) string {
	source = strings.TrimSpace(source)
	if ip := net.ParseIP(source); ip != nil {
		return ip.String()
	}
	_, network, _ := net.ParseCIDR(source)
	if network != nil {
		return network.String()
	}
	return source
}

func safeFirewallSourceIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}

func validateForwardIP(ip string) error {
	if net.ParseIP(strings.TrimSpace(ip)) == nil {
		return fmt.Errorf("invalid forward IP address %q", ip)
	}
	return nil
}

func validFirewallRuleID(id, prefix string) bool {
	value := strings.TrimPrefix(id, prefix)
	if value == id || value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func auditFirewall(action, objectID string) {
	log.Printf("[security-audit] firewall action=%s object=%s", action, objectID)
}

func (d *firewallData) initialize(ctx context.Context) error {
	d.initOnce.Do(func() {
		if runtime.GOOS != "linux" {
			d.initErr = fmt.Errorf("firewall management requires Linux")
			return
		}
		if err := d.load(); err != nil {
			d.initErr = err
			return
		}
		if err := d.ensureChains(ctx); err != nil {
			d.initErr = err
			return
		}
		d.initErr = d.reconcile(ctx)
	})
	return d.initErr
}

func (d *firewallData) load() error {
	data, err := os.ReadFile(d.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read firewall state: %w", err)
	}
	var state firewallState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parse firewall state: %w", err)
	}
	if state.Rules == nil {
		state.Rules = make(map[string]FirewallRule)
	}
	if state.Forwards == nil {
		state.Forwards = make(map[string]PortForward)
	}
	for id, rule := range state.Rules {
		if !validFirewallRuleID(id, "rule-") || rule.ID != id || rule.Port <= 0 || rule.Port > 65535 {
			return fmt.Errorf("invalid persisted firewall rule %q", id)
		}
		if _, err := validateFirewallProtocol(rule.Protocol); err != nil {
			return err
		}
		if _, err := validateFirewallAction(rule.Action); err != nil {
			return err
		}
		if err := validateFirewallSource(rule.SourceIP); err != nil {
			return err
		}
	}
	for id, forward := range state.Forwards {
		if !validFirewallRuleID(id, "fwd-") || forward.ID != id ||
			forward.FromPort <= 0 || forward.FromPort > 65535 || forward.ToPort <= 0 || forward.ToPort > 65535 {
			return fmt.Errorf("invalid persisted port forward %q", id)
		}
		if _, err := validateFirewallProtocol(forward.Protocol); err != nil {
			return err
		}
		if err := validateForwardIP(forward.ToIP); err != nil {
			return err
		}
	}
	d.state = state
	return nil
}

func (d *firewallData) persistLocked() error {
	path := d.path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create firewall state directory: %w", err)
	}
	data, err := json.MarshalIndent(d.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode firewall state: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".firewall-*.tmp")
	if err != nil {
		return fmt.Errorf("create firewall state temp file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace firewall state: %w", err)
	}
	return syncDirectory(filepath.Dir(path))
}

func commandExists(ctx context.Context, table, chain string) bool {
	args := []string{"-w"}
	if table != "filter" {
		args = append(args, "-t", table)
	}
	args = append(args, "-S", chain)
	return firewallExec(ctx, "iptables", args...) == nil
}

func ensureHook(ctx context.Context, table, parent, chain string) error {
	base := []string{"-w"}
	if table != "filter" {
		base = append(base, "-t", table)
	}
	check := append(append([]string{}, base...), "-C", parent, "-j", chain)
	if firewallExec(ctx, "iptables", check...) == nil {
		return nil
	}
	insert := append(append([]string{}, base...), "-I", parent, "1", "-j", chain)
	return firewallExec(ctx, "iptables", insert...)
}

func removeHook(ctx context.Context, table, parent, chain string) error {
	base := []string{"-w"}
	if table != "filter" {
		base = append(base, "-t", table)
	}
	check := append(append([]string{}, base...), "-C", parent, "-j", chain)
	if firewallExec(ctx, "iptables", check...) != nil {
		return nil
	}
	remove := append(append([]string{}, base...), "-D", parent, "-j", chain)
	return firewallExec(ctx, "iptables", remove...)
}

func (d *firewallData) ensureChains(ctx context.Context) error {
	if _, err := exec.LookPath("iptables"); err != nil {
		return errors.New("iptables is required for firewall management")
	}
	if !commandExists(ctx, "filter", firewallFilterChain) {
		if err := firewallExec(ctx, "iptables", "-w", "-N", firewallFilterChain); err != nil {
			return err
		}
	}
	if !commandExists(ctx, "nat", firewallNATChain) {
		if err := firewallExec(ctx, "iptables", "-w", "-t", "nat", "-N", firewallNATChain); err != nil {
			return err
		}
	}
	return nil
}

func ruleArgs(operation string, rule FirewallRule) []string {
	args := []string{"-w", operation, firewallFilterChain, "-p", rule.Protocol, "--dport", fmt.Sprint(rule.Port)}
	if rule.SourceIP != "" {
		args = append(args, "-s", rule.SourceIP)
	}
	return append(args, "-m", "comment", "--comment", "forge:"+rule.ID, "-j", "ACCEPT")
}

func forwardArgs(operation string, forward PortForward) []string {
	return []string{"-w", "-t", "nat", operation, firewallNATChain, "-p", forward.Protocol,
		"--dport", fmt.Sprint(forward.FromPort), "-m", "comment", "--comment", "forge:" + forward.ID,
		"-j", "DNAT", "--to-destination", net.JoinHostPort(forward.ToIP, fmt.Sprint(forward.ToPort))}
}

func (d *firewallData) reconcile(ctx context.Context) error {
	if d.state.Enabled {
		if err := ensureHook(ctx, "filter", "INPUT", firewallFilterChain); err != nil {
			return err
		}
		if err := ensureHook(ctx, "nat", "PREROUTING", firewallNATChain); err != nil {
			return err
		}
	} else {
		if err := removeHook(ctx, "filter", "INPUT", firewallFilterChain); err != nil {
			return err
		}
		if err := removeHook(ctx, "nat", "PREROUTING", firewallNATChain); err != nil {
			return err
		}
	}
	var filterRules strings.Builder
	filterRules.WriteString("*filter\n:")
	filterRules.WriteString(firewallFilterChain)
	filterRules.WriteString(" - [0:0]\n")
	ruleIDs := make([]string, 0, len(d.state.Rules))
	for id := range d.state.Rules {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)
	for _, id := range ruleIDs {
		rule := d.state.Rules[id]
		fmt.Fprintf(&filterRules, "-A %s -p %s --dport %d -s %s -m comment --comment forge:%s -j ACCEPT\n",
			firewallFilterChain, rule.Protocol, rule.Port, rule.SourceIP, rule.ID)
	}
	filterRules.WriteString("COMMIT\n")

	var natRules strings.Builder
	natRules.WriteString("*nat\n:")
	natRules.WriteString(firewallNATChain)
	natRules.WriteString(" - [0:0]\n")
	forwardIDs := make([]string, 0, len(d.state.Forwards))
	for id := range d.state.Forwards {
		forwardIDs = append(forwardIDs, id)
	}
	sort.Strings(forwardIDs)
	for _, id := range forwardIDs {
		forward := d.state.Forwards[id]
		fmt.Fprintf(&natRules, "-A %s -p %s --dport %d -m comment --comment forge:%s -j DNAT --to-destination %s\n",
			firewallNATChain, forward.Protocol, forward.FromPort, forward.ID,
			net.JoinHostPort(forward.ToIP, fmt.Sprint(forward.ToPort)))
	}
	natRules.WriteString("COMMIT\n")
	return firewallRestore(ctx, filterRules.String()+natRules.String())
}

func (s *Server) requireFirewall(w http.ResponseWriter, r *http.Request) bool {
	if err := s.firewall.initialize(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return false
	}
	return true
}

func (s *Server) handleFirewallStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireFirewall(w, r) {
		return
	}
	s.firewall.mu.RLock()
	enabled := s.firewall.state.Enabled
	s.firewall.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "backend": "iptables"})
}

func (s *Server) setFirewallEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	if !s.requireFirewall(w, r) {
		return
	}
	s.firewall.mu.Lock()
	previous := s.firewall.state.Enabled
	s.firewall.state.Enabled = enabled
	if err := s.firewall.reconcile(r.Context()); err != nil {
		s.firewall.state.Enabled = previous
		s.firewall.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := s.firewall.persistLocked(); err != nil {
		s.firewall.state.Enabled = previous
		_ = s.firewall.reconcile(r.Context())
		s.firewall.mu.Unlock()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.firewall.mu.Unlock()
	auditFirewall("set-enabled", fmt.Sprint(enabled))
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled})
}

func (s *Server) handleFirewallEnable(w http.ResponseWriter, r *http.Request) {
	s.setFirewallEnabled(w, r, true)
}

func (s *Server) handleFirewallDisable(w http.ResponseWriter, r *http.Request) {
	s.setFirewallEnabled(w, r, false)
}

func (s *Server) handleFirewallListRules(w http.ResponseWriter, r *http.Request) {
	if !s.requireFirewall(w, r) {
		return
	}
	s.firewall.mu.RLock()
	rules := make([]FirewallRule, 0, len(s.firewall.state.Rules))
	for _, rule := range s.firewall.state.Rules {
		rules = append(rules, rule)
	}
	s.firewall.mu.RUnlock()
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	writeJSON(w, http.StatusOK, rules)
}

func decodeFirewallRule(r *http.Request) (FirewallRule, error) {
	var rule FirewallRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		return rule, errors.New("invalid request body")
	}
	if rule.Port <= 0 || rule.Port > 65535 {
		return rule, errors.New("invalid port")
	}
	protocol, err := validateFirewallProtocol(rule.Protocol)
	if err != nil {
		return rule, err
	}
	action, err := validateFirewallAction(rule.Action)
	if err != nil {
		return rule, err
	}
	if err := validateFirewallSource(rule.SourceIP); err != nil {
		return rule, err
	}
	rule.Protocol = protocol
	rule.Action = action
	rule.SourceIP = canonicalFirewallSource(rule.SourceIP)
	return rule, nil
}

func (s *Server) handleFirewallAddRule(w http.ResponseWriter, r *http.Request) {
	if !s.requireFirewall(w, r) {
		return
	}
	rule, err := decodeFirewallRule(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rule.ID = "rule-" + uuid.NewString()
	rule.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.firewall.mu.Lock()
	defer s.firewall.mu.Unlock()
	if err := firewallExec(r.Context(), "iptables", ruleArgs("-A", rule)...); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	s.firewall.state.Rules[rule.ID] = rule
	if err := s.firewall.persistLocked(); err != nil {
		delete(s.firewall.state.Rules, rule.ID)
		_ = firewallExec(r.Context(), "iptables", ruleArgs("-D", rule)...)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	auditFirewall("add-rule", rule.ID)
	writeJSON(w, http.StatusCreated, rule)
}

func (s *Server) handleFirewallDeleteRule(w http.ResponseWriter, r *http.Request) {
	if !s.requireFirewall(w, r) {
		return
	}
	id := r.PathValue("id")
	s.firewall.mu.Lock()
	rule, ok := s.firewall.state.Rules[id]
	if !ok {
		s.firewall.mu.Unlock()
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if err := firewallExec(r.Context(), "iptables", ruleArgs("-D", rule)...); err != nil {
		s.firewall.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	delete(s.firewall.state.Rules, id)
	if err := s.firewall.persistLocked(); err != nil {
		s.firewall.state.Rules[id] = rule
		_ = firewallExec(r.Context(), "iptables", ruleArgs("-A", rule)...)
		s.firewall.mu.Unlock()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.firewall.mu.Unlock()
	auditFirewall("delete-rule", id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFirewallUpdateRule(w http.ResponseWriter, r *http.Request) {
	if !s.requireFirewall(w, r) {
		return
	}
	id := r.PathValue("id")
	updated, err := decodeFirewallRule(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.firewall.mu.Lock()
	previous, ok := s.firewall.state.Rules[id]
	if !ok {
		s.firewall.mu.Unlock()
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	updated.ID = id
	updated.CreatedAt = previous.CreatedAt
	if err := firewallExec(r.Context(), "iptables", ruleArgs("-D", previous)...); err != nil {
		s.firewall.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := firewallExec(r.Context(), "iptables", ruleArgs("-A", updated)...); err != nil {
		_ = firewallExec(r.Context(), "iptables", ruleArgs("-A", previous)...)
		s.firewall.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	s.firewall.state.Rules[id] = updated
	if err := s.firewall.persistLocked(); err != nil {
		s.firewall.state.Rules[id] = previous
		_ = firewallExec(r.Context(), "iptables", ruleArgs("-D", updated)...)
		_ = firewallExec(r.Context(), "iptables", ruleArgs("-A", previous)...)
		s.firewall.mu.Unlock()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.firewall.mu.Unlock()
	auditFirewall("update-rule", id)
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleFirewallPort(w http.ResponseWriter, r *http.Request) {
	s.handleFirewallAddRule(w, r)
}

func (s *Server) handleFirewallListForwards(w http.ResponseWriter, r *http.Request) {
	if !s.requireFirewall(w, r) {
		return
	}
	s.firewall.mu.RLock()
	forwards := make([]PortForward, 0, len(s.firewall.state.Forwards))
	for _, forward := range s.firewall.state.Forwards {
		forwards = append(forwards, forward)
	}
	s.firewall.mu.RUnlock()
	sort.Slice(forwards, func(i, j int) bool { return forwards[i].ID < forwards[j].ID })
	writeJSON(w, http.StatusOK, forwards)
}

func (s *Server) handleFirewallAddForward(w http.ResponseWriter, r *http.Request) {
	if !s.requireFirewall(w, r) {
		return
	}
	var forward PortForward
	if err := json.NewDecoder(r.Body).Decode(&forward); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if forward.FromPort <= 0 || forward.FromPort > 65535 || forward.ToPort <= 0 || forward.ToPort > 65535 {
		writeError(w, http.StatusBadRequest, "invalid port range")
		return
	}
	if err := validateForwardIP(forward.ToIP); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	protocol, err := validateFirewallProtocol(forward.Protocol)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	forward.Protocol = protocol
	forward.ToIP = net.ParseIP(strings.TrimSpace(forward.ToIP)).String()
	forward.ID = "fwd-" + uuid.NewString()
	forward.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	s.firewall.mu.Lock()
	defer s.firewall.mu.Unlock()
	if err := firewallExec(r.Context(), "iptables", forwardArgs("-A", forward)...); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	s.firewall.state.Forwards[forward.ID] = forward
	if err := s.firewall.persistLocked(); err != nil {
		delete(s.firewall.state.Forwards, forward.ID)
		_ = firewallExec(r.Context(), "iptables", forwardArgs("-D", forward)...)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	auditFirewall("add-forward", forward.ID)
	writeJSON(w, http.StatusCreated, forward)
}

func (s *Server) handleFirewallDeleteForward(w http.ResponseWriter, r *http.Request) {
	if !s.requireFirewall(w, r) {
		return
	}
	id := r.PathValue("id")
	s.firewall.mu.Lock()
	forward, ok := s.firewall.state.Forwards[id]
	if !ok {
		s.firewall.mu.Unlock()
		writeError(w, http.StatusNotFound, "forward not found")
		return
	}
	if err := firewallExec(r.Context(), "iptables", forwardArgs("-D", forward)...); err != nil {
		s.firewall.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	delete(s.firewall.state.Forwards, id)
	if err := s.firewall.persistLocked(); err != nil {
		s.firewall.state.Forwards[id] = forward
		_ = firewallExec(r.Context(), "iptables", forwardArgs("-A", forward)...)
		s.firewall.mu.Unlock()
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.firewall.mu.Unlock()
	auditFirewall("delete-forward", id)
	w.WriteHeader(http.StatusNoContent)
}
