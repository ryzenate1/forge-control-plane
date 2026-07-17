package auth

import (
	"strings"
)

type Scope string

const (
	ScopeServerRead  Scope = "server:read"
	ScopeServerWrite Scope = "server:write"
	ScopeBackupRead  Scope = "backup:read"
	ScopeBackupWrite Scope = "backup:write"
	ScopeAdmin       Scope = "admin"
)

type Scopes []Scope

func (s Scopes) Contains(scope Scope) bool {
	for _, s := range s {
		if s == scope {
			return true
		}
	}
	return false
}

func (s Scopes) Intersect(other Scopes) Scopes {
	var result Scopes
	for _, scope := range s {
		if other.Contains(scope) {
			result = append(result, scope)
		}
	}
	return result
}

func (s Scopes) String() string {
	ss := make([]string, len(s))
	for i, scope := range s {
		ss[i] = string(scope)
	}
	return strings.Join(ss, ", ")
}
