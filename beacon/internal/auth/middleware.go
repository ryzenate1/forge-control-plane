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
			if path == "/health" || path == "/metrics" || path == "/ready" {
				next.ServeHTTP(w, r)
				return
			}

			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "missing or invalid authorization header", http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(auth, "Bearer ")

			claims, err := gen.Validate(tokenStr)
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

func RequireScopes(scopes ...Scope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}

			tokenScope := Scope(claims.Scope)
			for _, s := range scopes {
				if tokenScope == s {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Error(w, "insufficient scope", http.StatusForbidden)
		})
	}
}
