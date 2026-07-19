package auth

import (
	"fmt"
	"sort"
	"strings"
)

const (
	ServerRead    = "server:read"
	ServerWrite   = "server:write"
	ServerAdmin   = "server:admin"
	ServerConsole = "server:console"
	ServerFiles   = "server:files"
	ServerBackups = "server:backups"

	UserRead  = "user:read"
	UserWrite = "user:write"
	UserAdmin = "user:admin"

	NodeRead  = "node:read"
	NodeWrite = "node:write"
	NodeAdmin = "node:admin"

	SystemRead  = "system:read"
	SystemAdmin = "system:admin"
	SystemAudit = "system:audit"
)

var AllExtendedScopes = []string{
	ServerRead, ServerWrite, ServerAdmin, ServerConsole, ServerFiles, ServerBackups,
	UserRead, UserWrite, UserAdmin,
	NodeRead, NodeWrite, NodeAdmin,
	SystemRead, SystemAdmin, SystemAudit,
}

type ExtendedScopeSet map[string]bool

func NewExtendedScopeSet(scopes ...string) ExtendedScopeSet {
	s := make(ExtendedScopeSet, len(scopes))
	for _, scope := range scopes {
		s[scope] = true
	}
	return s
}

func (s ExtendedScopeSet) Has(scope string) bool {
	return s[scope]
}

func (s ExtendedScopeSet) Add(scope string) {
	s[scope] = true
}

func (s ExtendedScopeSet) Remove(scope string) {
	delete(s, scope)
}

func (s ExtendedScopeSet) Merge(other ExtendedScopeSet) ExtendedScopeSet {
	result := make(ExtendedScopeSet, len(s)+len(other))
	for k, v := range s {
		if v {
			result[k] = true
		}
	}
	for k, v := range other {
		if v {
			result[k] = true
		}
	}
	return result
}

func (s ExtendedScopeSet) Intersect(other ExtendedScopeSet) ExtendedScopeSet {
	result := make(ExtendedScopeSet)
	for k := range s {
		if s[k] && other[k] {
			result[k] = true
		}
	}
	return result
}

func (s ExtendedScopeSet) IsSubsetOf(other ExtendedScopeSet) bool {
	for k := range s {
		if s[k] && !other[k] {
			return false
		}
	}
	return true
}

func (s ExtendedScopeSet) Slice() []string {
	result := make([]string, 0, len(s))
	for k := range s {
		if s[k] {
			result = append(result, k)
		}
	}
	sort.Strings(result)
	return result
}

func (s ExtendedScopeSet) String() string {
	return strings.Join(s.Slice(), ",")
}

var RoleScopes = map[string]ExtendedScopeSet{
	"admin": NewExtendedScopeSet(AllExtendedScopes...),
	"user": NewExtendedScopeSet(
		ServerRead, ServerConsole, ServerFiles, ServerBackups,
		UserRead, UserWrite,
		NodeRead,
		SystemRead,
	),
	"viewer": NewExtendedScopeSet(
		ServerRead,
		UserRead,
	),
}

func CheckScope(userScopes ExtendedScopeSet, required string) bool {
	if userScopes[required] {
		return true
	}
	parts := strings.SplitN(required, ":", 2)
	if len(parts) != 2 {
		return false
	}
	wildcard := parts[0] + ":*"
	if userScopes[wildcard] {
		return true
	}
	adminWildcard := parts[0] + ":admin"
	return userScopes[adminWildcard]
}

func RequireScope(userScopes ExtendedScopeSet, required string) bool {
	return CheckScope(userScopes, required)
}

func ValidateExtendedScopes(s ExtendedScopeSet) error {
	for k := range s {
		if !s[k] {
			continue
		}
		if strings.HasSuffix(k, ":*") || strings.HasSuffix(k, ".*") {
			continue
		}
		found := false
		for _, known := range AllExtendedScopes {
			if known == k {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("auth: unknown extended scope %q", k)
		}
	}
	return nil
}

func ExpandScopes(scopes []string) ExtendedScopeSet {
	result := make(ExtendedScopeSet)
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if strings.HasSuffix(scope, ":*") || strings.HasSuffix(scope, ".*") {
			prefix := strings.TrimSuffix(strings.TrimSuffix(scope, ":*"), ".*")
			for _, s := range AllExtendedScopes {
				if strings.HasPrefix(s, prefix+":") {
					result[s] = true
				}
			}
		} else {
			result[scope] = true
		}
	}
	return result
}
