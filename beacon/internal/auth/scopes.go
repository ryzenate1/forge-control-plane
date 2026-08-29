package auth

import (
	"errors"
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

// ParseScopes splits a scope claim into its individual grants. It is the only
// accepted way to read a scope list: an empty entry and any wildcard entry are
// dropped rather than kept, so a token that claims "*" or an unparseable list
// grants nothing instead of granting everything.
func ParseScopes(value string) Scopes {
	var parsed Scopes
	for _, field := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ';'
	}) {
		scope := Scope(strings.TrimSpace(field))
		if scope == "" || scope.Valid() != nil {
			continue
		}
		parsed = append(parsed, scope)
	}
	return parsed
}

// Valid refuses an empty scope and the wildcard spellings that would otherwise
// satisfy every requirement. It does not pin a vocabulary: an unknown but
// well-formed scope is the authorisation layer's problem, and it still has to
// match a requirement exactly to grant anything.
func (s Scope) Valid() error {
	value := strings.TrimSpace(string(s))
	if value == "" {
		return errEmptyScope
	}
	if value != string(s) {
		return errUntrimmedScope
	}
	switch strings.ToLower(value) {
	case "*", "all", "any", "*:*", "*.*", "scope:*":
		return errWildcardScope
	}
	if strings.HasPrefix(value, "!") || strings.ContainsAny(value, "\x00\n\r") {
		return errMalformedScope
	}
	return nil
}

var (
	errEmptyScope      = errors.New("scope is empty")
	errUntrimmedScope  = errors.New("scope has surrounding whitespace")
	errWildcardScope   = errors.New("scope is a wildcard")
	errMalformedScope  = errors.New("scope is malformed")
)

// Contains reports whether the exact scope was granted. Matching is equality
// only: there is no prefix or wildcard rule that would let "server:read"
// satisfy "server:write".
func (s Scopes) Contains(scope Scope) bool {
	if scope.Valid() != nil {
		return false
	}
	for _, candidate := range s {
		if candidate == scope {
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
