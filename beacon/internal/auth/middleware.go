package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"gamepanel/beacon/internal/tokens"
)

type contextKey string

const claimsKey contextKey = "auth-claims"

func ClaimsFromContext(ctx context.Context) *tokens.Claims {
	claims, _ := ctx.Value(claimsKey).(*tokens.Claims)
	return claims
}

func NewAuthMiddleware(gen *tokens.Generator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if r.Method == http.MethodGet && (path == "/health" || path == "/ready") {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := authenticateToken(gen, r)
			if err != nil {
				if errors.Is(err, tokens.ErrTokenExpired) {
					http.Error(w, "token expired", http.StatusUnauthorized)
					return
				}
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerToken returns the request's bearer credential. It accepts only a
// well-formed single-value "Bearer" scheme; a repeated Authorization header is
// ambiguous and is refused rather than resolved to whichever value the runtime
// happens to join.
func bearerToken(r *http.Request) (string, error) {
	values, ok := r.Header["Authorization"]
	if !ok {
		return "", errors.New("missing authorization header")
	}
	if len(values) != 1 {
		return "", errors.New("ambiguous authorization header")
	}
	auth := strings.TrimSpace(values[0])
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", errors.New("unsupported authorization scheme")
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if token == "" {
		return "", errors.New("empty bearer token")
	}
	return token, nil
}

// authenticateToken verifies a credential. It deliberately does not spend a
// one-time ticket: inspection must stay non-consuming so a handler can read the
// claims before an upgrade completes. Routes whose point of use *is* this
// request must use RedeemTicketMiddleware instead.
func authenticateToken(gen *tokens.Generator, r *http.Request) (*tokens.Claims, error) {
	if gen == nil {
		return nil, errors.New("token generator not configured")
	}
	tokenStr, err := bearerToken(r)
	if err != nil {
		return nil, err
	}
	return gen.Validate(tokenStr)
}

// grantedScopes splits a token's scope claim into the set of scopes it carries.
// An empty, whitespace-only or wildcard entry yields no grant at all, so a
// malformed or over-broad claim can never satisfy a requirement by accident.
func grantedScopes(claims *tokens.Claims) map[Scope]struct{} {
	granted := make(map[Scope]struct{})
	if claims == nil {
		return granted
	}
	for _, value := range strings.FieldsFunc(string(claims.Scope), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	}) {
		scope := Scope(strings.TrimSpace(value))
		if scope == "" || isWildcardScope(scope) {
			continue
		}
		granted[scope] = struct{}{}
	}
	return granted
}

func isWildcardScope(scope Scope) bool {
	switch strings.ToLower(string(scope)) {
	case "*", "all", "any", "scope:*", "*:*":
		return true
	default:
		return false
	}
}

// RequireScopes authorizes a request only when the token carries every listed
// scope. Listing no scope is a configuration mistake that would otherwise
// admit any authenticated credential, so it denies.
func RequireScopes(scopes ...Scope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			if len(scopes) == 0 {
				http.Error(w, "insufficient scope", http.StatusForbidden)
				return
			}

			granted := grantedScopes(claims)
			for _, required := range scopes {
				if _, ok := granted[required]; !ok {
					http.Error(w, "insufficient scope", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyScope authorizes a token when at least one requested scope is
// present. Use RequireScopes when every supplied scope is required. As with
// RequireScopes, an empty requirement list denies.
func RequireAnyScope(scopes ...Scope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			if len(scopes) == 0 {
				http.Error(w, "insufficient scope", http.StatusForbidden)
				return
			}
			accepted := make(map[Scope]struct{}, len(scopes))
			for _, scope := range scopes {
				accepted[scope] = struct{}{}
			}
			for granted := range grantedScopes(claims) {
				if _, ok := accepted[granted]; ok {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, "insufficient scope", http.StatusForbidden)
		})
	}
}

// RedeemTicketMiddleware authenticates a one-time ticket and spends it at the
// point of use, then requires the ticket to carry one of the accepted scopes.
//
// Validate-only authorization is not enough for a stream or a file transfer:
// the credential arrives in a query string, is visible to every proxy and
// access log in between, and stays replayable until it expires unless someone
// consumes it. Mount this on the handler that performs the work, in place of
// the handler's own call to Generator.Redeem - spending twice in one request
// must fail, and it does.
func RedeemTicketMiddleware(gen *tokens.Generator, accepted ...Scope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if gen == nil {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			if len(accepted) == 0 {
				http.Error(w, "insufficient scope", http.StatusForbidden)
				return
			}
			tokenStr, err := bearerTicket(r)
			if err != nil {
				http.Error(w, "missing or invalid ticket", http.StatusUnauthorized)
				return
			}
			claims, err := gen.Redeem(tokenStr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
			granted := grantedScopes(claims)
			allowed := false
			for _, scope := range accepted {
				if _, ok := granted[scope]; ok {
					allowed = true
					break
				}
			}
			if !allowed {
				http.Error(w, "insufficient scope", http.StatusForbidden)
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerTicket reads a ticket from the bearer header or, because browsers
// cannot set headers on a websocket upgrade, from the token query parameter.
// Both must not be present with different values: an ambiguous credential is
// refused, not resolved.
func bearerTicket(r *http.Request) (string, error) {
	query := strings.TrimSpace(r.URL.Query().Get("token"))
	header, headerErr := bearerToken(r)
	if headerErr != nil {
		if query == "" {
			return "", headerErr
		}
		return query, nil
	}
	if query != "" && query != header {
		return "", errors.New("conflicting ticket sources")
	}
	return header, nil
}

// RequireServerBinding refuses a resource-bound ticket whose server claim does
// not name the server the request is acting on. An empty claim on either side
// is a mismatch: a token bound to nothing must not be allowed to choose its own
// target at the handler's door.
func RequireServerBinding(pathParam string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			target := strings.TrimSpace(r.PathValue(pathParam))
			bound := strings.TrimSpace(claims.ServerID)
			if target == "" || bound == "" || target != bound {
				http.Error(w, "token is not bound to this server", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
