// Package vaultprovider integrates external HashiCorp Vault secret stores so
// that environment-variable values can reference a secret fetched live at
// deploy time instead of being duplicated into Forge's own encrypted store.
//
// It re-implements, natively and independently, the concept popularised by
// Dokploy's vault-provider router: register one or more Vault connections
// (endpoint + KV mount + auth method), then let an environment variable hold a
// reference such as `vault:<connection-id>/<secret-path>#<field>` which the
// control plane resolves against the named connection when building a
// deployment's environment.
package vaultprovider

import (
	"strings"
)

// ReferencePrefix marks an environment-variable value as a Vault reference
// rather than a literal secret. A value only enters the Vault path when it
// begins with this prefix; everything else is passed through unchanged.
const ReferencePrefix = "vault:"

// AuthMethod enumerates the supported Vault authentication methods.
type AuthMethod string

const (
	AuthToken   AuthMethod = "token"
	AuthAppRole AuthMethod = "approle"
)

// ParseAuthMethod normalizes a caller-supplied auth method string, rejecting
// unknown values instead of silently defaulting.
func ParseAuthMethod(method string) (AuthMethod, error) {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "token":
		return AuthToken, nil
	case "approle", "app-role", "app_role":
		return AuthAppRole, nil
	default:
		return "", ErrUnknownAuthMethod
	}
}

// VaultConnection is the service's live view of a registered connection, with
// decrypted credentials ready to talk to Vault. It is populated from the store
// and must never be serialized to a client.
type VaultConnection struct {
	ID            string
	Name          string
	BaseURL       string
	MountPath     string
	Namespace     string
	EngineVersion int
	Auth          AuthMethod
	Token         string
	RoleID        string
	SecretID      string
	Enabled       bool
}

// ConnectionInput is the create/edit payload from the admin API. Credential
// fields are optional so an edit that omits them preserves the stored secret.
type ConnectionInput struct {
	Name          string
	BaseURL       string
	MountPath     string
	Namespace     *string
	EngineVersion int
	AuthMethod    string
	Token         *string
	RoleID        *string
	SecretID      *string
	Enabled       *bool
	OrgID         *string
}

// VaultRef is a parsed environment-variable Vault reference.
type VaultRef struct {
	ConnectionID string
	Path         string
	Field        string
}

// ParseVaultReference reports whether value is a Vault reference and, if so,
// decomposes it. ok is true whenever the value carries the reference prefix —
// even if the remainder is structurally invalid — so a malformed reference is
// routed to the resolver and surfaces as an error rather than silently passing
// through as a literal secret. The accepted syntax is
//
//	vault:<connection-id>/<secret-path>#<field>
//
// where <secret-path> may itself contain slashes and <field> names the key
// within the KV secret.
func ParseVaultReference(value string) (VaultRef, bool) {
	if !strings.HasPrefix(value, ReferencePrefix) {
		return VaultRef{}, false
	}
	rest := strings.TrimPrefix(value, ReferencePrefix)
	var ref VaultRef
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		// Only a connection id (or nothing) — treat as a reference with an empty
		// path so validation in the resolver rejects it loudly.
		ref.ConnectionID = rest
		return ref, true
	}
	ref.ConnectionID = rest[:slash]
	remainder := rest[slash+1:]
	if hash := strings.LastIndexByte(remainder, '#'); hash >= 0 {
		ref.Path = remainder[:hash]
		ref.Field = remainder[hash+1:]
	} else {
		ref.Path = remainder
	}
	return ref, true
}

// Valid reports whether every component of the reference is present.
func (r VaultRef) Valid() bool {
	return r.ConnectionID != "" && r.Path != "" && r.Field != ""
}

// RenderVaultReference reconstructs the canonical reference string from its
// parts. It is the inverse of ParseVaultReference for well-formed refs and is
// used for echoing a normalized reference back to callers (never for leaking
// the resolved secret).
func RenderVaultReference(r VaultRef) string {
	var b strings.Builder
	b.WriteString(ReferencePrefix)
	b.WriteString(r.ConnectionID)
	b.WriteString("/")
	b.WriteString(r.Path)
	b.WriteString("#")
	b.WriteString(r.Field)
	return b.String()
}
