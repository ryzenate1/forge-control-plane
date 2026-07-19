package server

import (
	"io"
	"log"
	"net/http"
)

// writeError sends a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// readAllBounded reads up to max bytes from r and returns the body. Used by
// Wings-style update/deauthorize-user endpoints that may carry modest JSON
// payloads.
func readAllBounded(r io.ReadCloser, max int64) ([]byte, error) {
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, max))
}

// slogUpdateAccepted logs that the panel pushed an update payload to this
// daemon. The list of top-level keys is included for traceability.
func slogUpdateAccepted(keys []string) {
	log.Printf("config update accepted: %d top-level keys (%v)", len(keys), keys)
}
