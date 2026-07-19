package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteError(t *testing.T) {
	t.Run("defaults title to StatusText", func(t *testing.T) {
		w := httptest.NewRecorder()
		WriteError(w, http.StatusNotFound, "not_found", "")
		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != APIContentType+"; charset=utf-8" {
			t.Errorf("expected Content-Type %q, got %q", APIContentType+"; charset=utf-8", ct)
		}

		var er ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(er.Errors) != 1 {
			t.Fatalf("expected 1 error, got %d", len(er.Errors))
		}
		if er.Errors[0].Title != "Not Found" {
			t.Errorf("expected title 'Not Found', got %q", er.Errors[0].Title)
		}
	})

	t.Run("uses provided title and detail", func(t *testing.T) {
		w := httptest.NewRecorder()
		WriteError(w, http.StatusBadRequest, "bad_request", "Bad Request", "missing field")
		resp := w.Result()
		defer resp.Body.Close()

		var er ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if er.Errors[0].Code != "bad_request" {
			t.Errorf("expected code 'bad_request', got %q", er.Errors[0].Code)
		}
		if er.Errors[0].Title != "Bad Request" {
			t.Errorf("expected title 'Bad Request', got %q", er.Errors[0].Title)
		}
		if er.Errors[0].Detail != "missing field" {
			t.Errorf("expected detail 'missing field', got %q", er.Errors[0].Detail)
		}
		if er.Errors[0].Status != 0 {
			t.Errorf("expected Status not to be serialized, got %d", er.Errors[0].Status)
		}
	})

	t.Run("only first detail is used", func(t *testing.T) {
		w := httptest.NewRecorder()
		WriteError(w, http.StatusInternalServerError, "err", "", "first", "second")
		resp := w.Result()
		defer resp.Body.Close()

		var er ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if er.Errors[0].Detail != "first" {
			t.Errorf("expected detail 'first', got %q", er.Errors[0].Detail)
		}
	})
}

func TestWriteAPIErrors(t *testing.T) {
	t.Run("writes multiple errors", func(t *testing.T) {
		w := httptest.NewRecorder()
		WriteAPIErrors(w, http.StatusBadRequest,
			APIError{Code: "err1", Title: "First Error", Status: http.StatusBadRequest},
			APIError{Code: "err2", Title: "Second Error", Status: http.StatusBadRequest},
		)
		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
		var er ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(er.Errors) != 2 {
			t.Fatalf("expected 2 errors, got %d", len(er.Errors))
		}
	})

	t.Run("empty errors falls back to internal_error", func(t *testing.T) {
		w := httptest.NewRecorder()
		WriteAPIErrors(w, http.StatusForbidden)
		resp := w.Result()
		defer resp.Body.Close()

		var er ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(er.Errors) != 1 {
			t.Fatalf("expected 1 fallback error, got %d", len(er.Errors))
		}
		if er.Errors[0].Code != "internal_error" {
			t.Errorf("expected code 'internal_error', got %q", er.Errors[0].Code)
		}
		if er.Errors[0].Title != "Forbidden" {
			t.Errorf("expected title 'Forbidden', got %q", er.Errors[0].Title)
		}
	})
}

func TestErrBadRequest(t *testing.T) {
	err := ErrBadRequest("bad_req", "Bad Request")
	if err.Code != "bad_req" {
		t.Errorf("expected code 'bad_req', got %q", err.Code)
	}
	if err.Title != "Bad Request" {
		t.Errorf("expected title 'Bad Request', got %q", err.Title)
	}
	if err.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", err.Status)
	}
}

func TestErrUnauthorized(t *testing.T) {
	err := ErrUnauthorized("unauth", "Unauthorized")
	if err.Status != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", err.Status)
	}
}

func TestErrNotFound(t *testing.T) {
	err := ErrNotFound("not_found", "Not Found")
	if err.Status != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", err.Status)
	}
}

func TestErrInternal(t *testing.T) {
	err := ErrInternal("internal", "Internal Error")
	if err.Status != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", err.Status)
	}
}

func TestErrConflict(t *testing.T) {
	err := ErrConflict("conflict", "Conflict")
	if err.Status != http.StatusConflict {
		t.Errorf("expected status 409, got %d", err.Status)
	}
}

func TestErrForbidden(t *testing.T) {
	err := ErrForbidden("forbidden", "Forbidden")
	if err.Status != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", err.Status)
	}
}

func TestErrTooManyRequests(t *testing.T) {
	err := ErrTooManyRequests("rate_limit", "Too Many Requests")
	if err.Status != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", err.Status)
	}
}

func TestAPIErrorCollector(t *testing.T) {
	t.Run("nil receiver is safe", func(t *testing.T) {
		var c *APIErrorCollector
		c.Add(ErrBadRequest("code", "title"))
		if c.HasErrors() {
			t.Error("expected HasErrors to be false for nil receiver")
		}
		if errs := c.Errors(); errs != nil {
			t.Errorf("expected nil errors, got %v", errs)
		}
		w := httptest.NewRecorder()
		c.Flush(w)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", w.Code)
		}
	})

	t.Run("Add, HasErrors, Errors", func(t *testing.T) {
		var c APIErrorCollector
		if c.HasErrors() {
			t.Error("expected HasErrors to be false initially")
		}
		c.Add(ErrBadRequest("code", "title"))
		if !c.HasErrors() {
			t.Error("expected HasErrors to be true after Add")
		}
		if errs := c.Errors(); len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d", len(errs))
		}
	})

	t.Run("Flush writes accumulated errors", func(t *testing.T) {
		var c APIErrorCollector
		c.Add(ErrBadRequest("code", "title"))
		w := httptest.NewRecorder()
		c.Flush(w)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		var er ErrorResponse
		if err := json.NewDecoder(w.Body).Decode(&er); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(er.Errors) != 1 {
			t.Fatalf("expected 1 error, got %d", len(er.Errors))
		}
	})

	t.Run("Flush with no errors writes fallback", func(t *testing.T) {
		var c APIErrorCollector
		w := httptest.NewRecorder()
		c.Flush(w)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", w.Code)
		}
		var er ErrorResponse
		if err := json.NewDecoder(w.Body).Decode(&er); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(er.Errors) != 1 {
			t.Fatalf("expected 1 fallback error, got %d", len(er.Errors))
		}
		if er.Errors[0].Code != "internal_error" {
			t.Errorf("expected code 'internal_error', got %q", er.Errors[0].Code)
		}
	})

	t.Run("Flush uses status of first error with non-zero Status", func(t *testing.T) {
		var c APIErrorCollector
		c.Add(APIError{Code: "c1", Title: "t1", Status: 0})
		c.Add(APIError{Code: "c2", Title: "t2", Status: http.StatusTeapot})
		w := httptest.NewRecorder()
		c.Flush(w)
		if w.Code != http.StatusTeapot {
			t.Errorf("expected 418, got %d", w.Code)
		}
	})
}

func TestNewError(t *testing.T) {
	t.Run("without details", func(t *testing.T) {
		e := NewError(404, "not found")
		if e.Code != 404 {
			t.Errorf("expected code 404, got %d", e.Code)
		}
		if e.Message != "not found" {
			t.Errorf("expected message 'not found', got %q", e.Message)
		}
		if e.Details != "" {
			t.Errorf("expected empty details, got %q", e.Details)
		}
		msg := e.Error()
		if !strings.Contains(msg, "API Error 404: not found") {
			t.Errorf("unexpected Error() string: %q", msg)
		}
	})

	t.Run("with details", func(t *testing.T) {
		e := NewError(500, "internal", "something broke")
		if e.Details != "something broke" {
			t.Errorf("expected details 'something broke', got %q", e.Details)
		}
	})

	t.Run("only first detail is used", func(t *testing.T) {
		e := NewError(400, "bad", "first", "second")
		if e.Details != "first" {
			t.Errorf("expected details 'first', got %q", e.Details)
		}
	})
}
