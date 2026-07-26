package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

// writeError sends a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// limitBody wraps r.Body in an http.MaxBytesReader so that decodeJSONBody
// (or any other body reader) fails fast instead of allowing an unbounded
// read into memory.
func limitBody(w http.ResponseWriter, r *http.Request, maxBytes int64) io.ReadCloser {
	return http.MaxBytesReader(w, r.Body, maxBytes)
}

// decodeJSONBody decodes r's JSON body into dst, bounding the read to
// maxBytes via limitBody. Unlike a bare json.NewDecoder(r.Body).Decode,
// this distinguishes a too-large payload from malformed JSON: if the body
// exceeded maxBytes, it writes an HTTP 413 Payload Too Large response
// directly (with a distinct error message) and returns a non-nil error, so
// callers only need to handle the "already responded, just return" case.
//
// http.MaxBytesReader (since Go 1.19) returns an *http.MaxBytesError when
// the limit is exceeded, which we detect with errors.As. As a fallback for
// wrapped/older error shapes, we also match on the well-known error string
// used by net/http ("http: request body too large").
func decodeJSONBody(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) error {
	r.Body = limitBody(w, r, maxBytes)
	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || strings.Contains(err.Error(), "http: request body too large") {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return err
		}
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return err
	}
	return nil
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
