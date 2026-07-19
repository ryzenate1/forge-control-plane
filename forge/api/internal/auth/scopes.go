package auth

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Scope describes a single authorization grant attached to a token, session, or
// API key. Scopes are namespaced as "<resource>:<action>" (e.g. "server:read").
type Scope struct {
	Name        string
	Description string
	IsDefault   bool
}

// BuiltinScopes are the canonical scopes shipped with the panel. They cover the
// connectivity surface exposed by the panel API and the daemon control plane.
var BuiltinScopes = []Scope{
	{Name: "user:read", Description: "Read own user profile and preferences", IsDefault: true},
	{Name: "user:write", Description: "Update own user profile and credentials", IsDefault: true},
	{Name: "server:read", Description: "Read server details and state", IsDefault: true},
	{Name: "server:write", Description: "Update server configuration and settings", IsDefault: false},
	{Name: "server:control", Description: "Start, stop, restart, and send commands to servers", IsDefault: false},
	{Name: "backup:read", Description: "List and inspect server backups", IsDefault: true},
	{Name: "backup:write", Description: "Create, restore, and delete server backups", IsDefault: false},
	{Name: "file:read", Description: "Read files from server filesystems", IsDefault: true},
	{Name: "file:write", Description: "Write, upload, and delete files on server filesystems", IsDefault: false},
	{Name: "admin:read", Description: "Read panel administration data", IsDefault: false},
	{Name: "admin:write", Description: "Mutate panel administration data", IsDefault: false},
	{Name: "node:read", Description: "Read node/Beacon configuration and status", IsDefault: false},
	{Name: "node:write", Description: "Update node/Beacon configuration", IsDefault: false},
	{Name: "node:admin", Description: "Full administrative access to nodes", IsDefault: false},
	{Name: "server:admin", Description: "Full administrative access to servers", IsDefault: false},
	{Name: "server:console", Description: "Send commands to server console", IsDefault: false},
	{Name: "server:files", Description: "Access and manage server files", IsDefault: false},
	{Name: "server:backups", Description: "Manage server backups", IsDefault: false},
	{Name: "user:admin", Description: "Full administrative access to users", IsDefault: false},
	{Name: "system:read", Description: "Read system configuration and status", IsDefault: false},
	{Name: "system:admin", Description: "Full administrative access to system", IsDefault: false},
	{Name: "system:audit", Description: "View system audit logs", IsDefault: false},
}

// KnownScopes is the set of registered scope names, populated from BuiltinScopes.
// Callers may extend it at process start by mutating the map; mutations should
// happen before any goroutine begins parsing scopes.
var KnownScopes = func() map[string]Scope {
	m := make(map[string]Scope, len(BuiltinScopes))
	for _, s := range BuiltinScopes {
		m[s.Name] = s
	}
	return m
}()

// ScopeSet is a set of scope names.
type ScopeSet map[string]struct{}

// ParseScopes parses a space- or comma-separated scope list into a ScopeSet.
// Unknown scopes produce an error.
func ParseScopes(s string) (ScopeSet, error) {
	set := ScopeSet{}
	if s == "" {
		return set, nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n'
	})
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if _, ok := KnownScopes[f]; !ok {
			return nil, fmt.Errorf("auth: unknown scope %q", f)
		}
		set[f] = struct{}{}
	}
	return set, nil
}

// Has reports whether the set contains the given scope.
func (s ScopeSet) Has(scope string) bool {
	_, ok := s[scope]
	return ok
}

// HasAny reports whether the set contains at least one of the given scopes.
func (s ScopeSet) HasAny(scopes ...string) bool {
	for _, scope := range scopes {
		if _, ok := s[scope]; ok {
			return true
		}
	}
	return false
}

// HasAll reports whether the set contains all of the given scopes.
func (s ScopeSet) HasAll(scopes ...string) bool {
	for _, scope := range scopes {
		if _, ok := s[scope]; !ok {
			return false
		}
	}
	return true
}

// Union returns a new ScopeSet containing scopes from both sets.
func (s ScopeSet) Union(other ScopeSet) ScopeSet {
	out := make(ScopeSet, len(s)+len(other))
	for k := range s {
		out[k] = struct{}{}
	}
	for k := range other {
		out[k] = struct{}{}
	}
	return out
}

// String returns the scopes as a sorted, comma-joined string. The output is
// deterministic.
func (s ScopeSet) String() string {
	if len(s) == 0 {
		return ""
	}
	names := make([]string, 0, len(s))
	for k := range s {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// DefaultScopes returns a ScopeSet containing every builtin scope marked
// IsDefault.
func DefaultScopes() ScopeSet {
	set := ScopeSet{}
	for _, s := range BuiltinScopes {
		if s.IsDefault {
			set[s.Name] = struct{}{}
		}
	}
	return set
}

// Validate returns an error if the set contains any scope that is not in
// KnownScopes.
func Validate(s ScopeSet) error {
	for k := range s {
		if _, ok := KnownScopes[k]; !ok {
			return fmt.Errorf("%w: %q", ErrUnknownScope, k)
		}
	}
	return nil
}

// ErrUnknownScope is returned when a scope is not in KnownScopes.
var ErrUnknownScope = errors.New("auth: unknown scope")
