// LIMITATION: Firewall rules are stored in memory only and are never applied to the OS-level firewall
// (iptables/pfctl). The enable/disable/status endpoints interact with the system firewall, but
// individual rules and port forwards exist only in this process's memory. OS-level firewall
// application is not implemented on this node.
package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"
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

type firewallData struct {
	mu       sync.RWMutex
	enabled  bool
	rules    map[string]FirewallRule
	forwards map[string]PortForward
	idSeq    int64
}

var fwd = &firewallData{
	rules:    make(map[string]FirewallRule),
	forwards: make(map[string]PortForward),
	enabled:  true,
	idSeq:    1,
}

func isMacOS() bool {
	return runtime.GOOS == "darwin"
}

func execCmd(name string, args ...string) error {
	var stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stderr = &stderr
	return cmd.Run()
}

func systemFirewallEnabled() bool {
	if isMacOS() {
		return execCmd("pfctl", "-s", "info") == nil
	}
	return execCmd("iptables", "-L") == nil
}

func (s *Server) handleFirewallStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"enabled": systemFirewallEnabled()})
}

func (s *Server) handleFirewallEnable(w http.ResponseWriter, _ *http.Request) {
	if isMacOS() {
		execCmd("pfctl", "-e")
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true})
}

func (s *Server) handleFirewallDisable(w http.ResponseWriter, _ *http.Request) {
	if isMacOS() {
		execCmd("pfctl", "-d")
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
}

func (s *Server) handleFirewallListRules(w http.ResponseWriter, _ *http.Request) {
	fwd.mu.RLock()
	rules := make([]FirewallRule, 0, len(fwd.rules))
	for _, r := range fwd.rules {
		rules = append(rules, r)
	}
	fwd.mu.RUnlock()
	writeJSON(w, http.StatusOK, rules)
}

func (s *Server) handleFirewallAddRule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Port        int    `json:"port"`
		Protocol    string `json:"protocol"`
		SourceIP    string `json:"sourceIp"`
		Action      string `json:"action"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Port <= 0 || body.Port > 65535 {
		writeError(w, http.StatusBadRequest, "invalid port")
		return
	}
	if body.Protocol == "" {
		body.Protocol = "tcp"
	}
	if body.Action == "" {
		body.Action = "allow"
	}

	fwd.mu.Lock()
	fwd.idSeq++
	id := fmt.Sprintf("rule-%d", fwd.idSeq)
	rule := FirewallRule{
		ID:          id,
		Port:        body.Port,
		Protocol:    body.Protocol,
		SourceIP:    body.SourceIP,
		Action:      body.Action,
		Description: body.Description,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	fwd.rules[id] = rule
	fwd.mu.Unlock()
	log.Printf("[beacon] firewall rule %s stored in memory; OS-level firewall application not implemented on this node", id)
	writeJSON(w, http.StatusCreated, map[string]any{"status": "stored", "message": "rule stored in memory; OS-level firewall application not implemented on this node", "rule": rule})
}

func (s *Server) handleFirewallDeleteRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	fwd.mu.Lock()
	delete(fwd.rules, id)
	fwd.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFirewallUpdateRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Port        *int    `json:"port"`
		Protocol    *string `json:"protocol"`
		SourceIP    *string `json:"sourceIp"`
		Action      *string `json:"action"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	fwd.mu.Lock()
	rule, ok := fwd.rules[id]
	if !ok {
		fwd.mu.Unlock()
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if body.Port != nil {
		rule.Port = *body.Port
	}
	if body.Protocol != nil {
		rule.Protocol = *body.Protocol
	}
	if body.SourceIP != nil {
		rule.SourceIP = *body.SourceIP
	}
	if body.Action != nil {
		rule.Action = *body.Action
	}
	if body.Description != nil {
		rule.Description = *body.Description
	}
	fwd.rules[id] = rule
	fwd.mu.Unlock()
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleFirewallPort(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
		Action   string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Port <= 0 || body.Port > 65535 {
		writeError(w, http.StatusBadRequest, "invalid port")
		return
	}
	if body.Protocol == "" {
		body.Protocol = "tcp"
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "error", "message": "firewall port management not implemented on this node"})
}

func (s *Server) handleFirewallListForwards(w http.ResponseWriter, _ *http.Request) {
	fwd.mu.RLock()
	forwards := make([]PortForward, 0, len(fwd.forwards))
	for _, f := range fwd.forwards {
		forwards = append(forwards, f)
	}
	fwd.mu.RUnlock()
	writeJSON(w, http.StatusOK, forwards)
}

func (s *Server) handleFirewallAddForward(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FromPort    int    `json:"fromPort"`
		ToPort      int    `json:"toPort"`
		ToIP        string `json:"toIp"`
		Protocol    string `json:"protocol"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.FromPort <= 0 || body.FromPort > 65535 || body.ToPort <= 0 || body.ToPort > 65535 {
		writeError(w, http.StatusBadRequest, "invalid port range")
		return
	}
	if body.ToIP == "" {
		writeError(w, http.StatusBadRequest, "toIp is required")
		return
	}
	if body.Protocol == "" {
		body.Protocol = "tcp"
	}

	fwd.mu.Lock()
	fwd.idSeq++
	id := fmt.Sprintf("fwd-%d", fwd.idSeq)
	pf := PortForward{
		ID:          id,
		FromPort:    body.FromPort,
		ToPort:      body.ToPort,
		ToIP:        body.ToIP,
		Protocol:    body.Protocol,
		Description: body.Description,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	fwd.forwards[id] = pf
	fwd.mu.Unlock()
	writeJSON(w, http.StatusCreated, pf)
}

func (s *Server) handleFirewallDeleteForward(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	fwd.mu.Lock()
	delete(fwd.forwards, id)
	fwd.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
