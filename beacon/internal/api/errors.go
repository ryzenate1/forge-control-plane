package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// APIContentType is the JSON:API media type used by all error responses
// produced by this package.
const APIContentType = "application/vnd.api+json"

// ErrorResponse is the top-level JSON:API error document returned by the
// scaffold. Multiple errors are accumulated by APIErrorCollector before being
// flushed in a single response.
type ErrorResponse struct {
	Errors []APIError `json:"errors"`
}

// APIError describes a single error returned to the client. Status is the
// HTTP status code associated with the error; it is not serialized so that
// the surrounding HTTP response line carries the canonical status.
type APIError struct {
	Code   string       `json:"code"`
	Title  string       `json:"title"`
	Detail string       `json:"detail,omitempty"`
	Status int          `json:"-"`
	Source *ErrorSource `json:"source,omitempty"`
}

// ErrorSource points at the offending request field or parameter.
type ErrorSource struct {
	Pointer   string `json:"pointer,omitempty"`
	Parameter string `json:"parameter,omitempty"`
}

// WriteError serializes a single APIError to w at the supplied status. When
// details are provided the first one is used as the Detail field; an empty
// title falls back to http.StatusText(status).
func WriteError(w http.ResponseWriter, status int, code, title string, details ...string) {
	if title == "" {
		title = http.StatusText(status)
	}
	detail := ""
	if len(details) > 0 {
		detail = details[0]
	}
	WriteAPIErrors(w, status, APIError{
		Code:   code,
		Title:  title,
		Detail: detail,
		Status: status,
	})
}

// WriteAPIErrors serializes one or more APIErrors using status as the HTTP
// response status. It is the single encode path used by WriteError and
// APIErrorCollector.Flush.
func WriteAPIErrors(w http.ResponseWriter, status int, errs ...APIError) {
	if len(errs) == 0 {
		errs = []APIError{{Code: "internal_error", Title: http.StatusText(status), Status: status}}
	}
	w.Header().Set("Content-Type", APIContentType+"; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Errors: errs})
}

// Constructor helpers return APIError values pre-seeded with the canonical
// HTTP status so callers can attach source/detail before flushing.

// ErrBadRequest returns a 400 APIError.
func ErrBadRequest(code, title string) APIError {
	return APIError{Code: code, Title: title, Status: http.StatusBadRequest}
}

// ErrUnauthorized returns a 401 APIError.
func ErrUnauthorized(code, title string) APIError {
	return APIError{Code: code, Title: title, Status: http.StatusUnauthorized}
}

// ErrForbidden returns a 403 APIError.
func ErrForbidden(code, title string) APIError {
	return APIError{Code: code, Title: title, Status: http.StatusForbidden}
}

// ErrNotFound returns a 404 APIError.
func ErrNotFound(code, title string) APIError {
	return APIError{Code: code, Title: title, Status: http.StatusNotFound}
}

// ErrConflict returns a 409 APIError.
func ErrConflict(code, title string) APIError {
	return APIError{Code: code, Title: title, Status: http.StatusConflict}
}

// ErrInternal returns a 500 APIError.
func ErrInternal(code, title string) APIError {
	return APIError{Code: code, Title: title, Status: http.StatusInternalServerError}
}

// ErrTooManyRequests returns a 429 APIError.
func ErrTooManyRequests(code, title string) APIError {
	return APIError{Code: code, Title: title, Status: http.StatusTooManyRequests}
}

// APIErrorCollector accumulates APIError values emitted by validators or
// handlers and flushes them as a single JSON:API document. The first added
// error's Status is used as the HTTP response status when non-zero.
type APIErrorCollector struct {
	errs []APIError
}

// Add appends an APIError. Safe to call on a nil receiver.
func (c *APIErrorCollector) Add(err APIError) {
	if c == nil {
		return
	}
	c.errs = append(c.errs, err)
}

// HasErrors reports whether the collector has accumulated any errors.
func (c *APIErrorCollector) HasErrors() bool {
	return c != nil && len(c.errs) > 0
}

// Errors returns the accumulated errors. The returned slice is owned by the
// collector.
func (c *APIErrorCollector) Errors() []APIError {
	if c == nil {
		return nil
	}
	return c.errs
}

// Flush writes the accumulated errors to w. When no errors are present a
// single internal_error response is emitted. The HTTP status is derived
// from the first error with a non-zero Status, defaulting to 500.
func (c *APIErrorCollector) Flush(w http.ResponseWriter) {
	status := http.StatusInternalServerError
	errs := c.Errors()
	if len(errs) == 0 {
		WriteAPIErrors(w, status)
		return
	}
	for _, e := range errs {
		if e.Status != 0 {
			status = e.Status
			break
		}
	}
	WriteAPIErrors(w, status, errs...)
}

// Error represents an API error
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Error returns the error message
func (e *Error) Error() string {
	return fmt.Sprintf("API Error %d: %s", e.Code, e.Message)
}

// NewError creates a new API error
func NewError(code int, message string, details ...string) *Error {
	var detail string
	if len(details) > 0 {
		detail = details[0]
	}
	return &Error{
		Code:    code,
		Message: message,
		Details: detail,
	}
}

// ErrorHandler handles API errors
func ErrorHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Implement error handling logic
		next.ServeHTTP(w, r)
	})
}
