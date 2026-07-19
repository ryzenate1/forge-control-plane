package api

import (
	"net/http"
)

type Validator interface {
	Validate() error
}

func ValidateRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Implement request validation logic
		next.ServeHTTP(w, r)
	})
}
