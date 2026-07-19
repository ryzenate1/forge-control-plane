package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"gamepanel/beacon/internal/database"
	"gamepanel/beacon/internal/models"

	"github.com/gorilla/mux"
)

const maxRequestBodySize = 10 << 20 // 10 MB

type ServerHandler struct {
	db database.Database
}

func NewServerHandler(db database.Database) *ServerHandler {
	return &ServerHandler{db: db}
}

func (h *ServerHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/servers", h.ListServers).Methods(http.MethodGet)
	r.HandleFunc("/servers", h.CreateServer).Methods(http.MethodPost)
	r.HandleFunc("/servers/{id}", h.GetServer).Methods(http.MethodGet)
	r.HandleFunc("/servers/{id}", h.UpdateServer).Methods(http.MethodPut)
	r.HandleFunc("/servers/{id}", h.DeleteServer).Methods(http.MethodDelete)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
	}
}

func writeServerError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (h *ServerHandler) ListServers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	defer r.Body.Close()

	rows, err := h.db.Query(ctx, "SELECT id, name, node_id, status, created_at, updated_at FROM servers")
	if err != nil {
		log.Printf("list servers query error: %v", err)
		writeServerError(w, http.StatusInternalServerError, "failed to list servers")
		return
	}
	defer rows.Close()

	var servers []models.Server
	for rows.Next() {
		var s models.Server
		var createdAt, updatedAt string

		if err := rows.Scan(&s.ID, &s.Name, &s.NodeID, &s.Status, &createdAt, &updatedAt); err != nil {
			log.Printf("list servers scan error: %v", err)
			writeServerError(w, http.StatusInternalServerError, "failed to read server data")
			return
		}

		s.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		servers = append(servers, s)
	}

	if err := rows.Err(); err != nil {
		log.Printf("list servers rows iteration error: %v", err)
		writeServerError(w, http.StatusInternalServerError, "failed to iterate server results")
		return
	}

	if servers == nil {
		servers = []models.Server{}
	}

	writeJSON(w, http.StatusOK, servers)
}

func (h *ServerHandler) CreateServer(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	// Validate Content-Type
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeServerError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	var server models.Server
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&server); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxErr):
			writeServerError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON at byte %d", syntaxErr.Offset))
		case errors.As(err, &unmarshalErr):
			writeServerError(w, http.StatusBadRequest, fmt.Sprintf("invalid value for field %q", unmarshalErr.Field))
		case errors.Is(err, io.ErrUnexpectedEOF):
			writeServerError(w, http.StatusBadRequest, "truncated JSON body")
		default:
			writeServerError(w, http.StatusBadRequest, "invalid request body")
		}
		return
	}

	// Validate required fields
	if strings.TrimSpace(server.ID) == "" {
		writeServerError(w, http.StatusBadRequest, "id is required")
		return
	}
	if strings.TrimSpace(server.Name) == "" {
		writeServerError(w, http.StatusBadRequest, "name is required")
		return
	}
	if strings.TrimSpace(server.NodeID) == "" {
		writeServerError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if strings.TrimSpace(string(server.Status)) == "" {
		server.Status = models.ServerStatusStopped
	}
	if !isValidServerStatus(server.Status) {
		writeServerError(w, http.StatusBadRequest, fmt.Sprintf("invalid status %q", server.Status))
		return
	}

	now := time.Now().UTC()
	server.CreatedAt = now
	server.UpdatedAt = now

	ctx := r.Context()

	_, err := h.db.Exec(ctx,
		"INSERT INTO servers (id, name, node_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		server.ID, server.Name, server.NodeID, server.Status, server.CreatedAt.Format(time.RFC3339), server.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		log.Printf("create server db error: %v", err)
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			writeServerError(w, http.StatusConflict, fmt.Sprintf("server with id %q already exists", server.ID))
			return
		}
		writeServerError(w, http.StatusInternalServerError, "failed to create server")
		return
	}

	writeJSON(w, http.StatusCreated, server)
}

func (h *ServerHandler) GetServer(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	vars := mux.Vars(r)
	id := strings.TrimSpace(vars["id"])
	if id == "" {
		writeServerError(w, http.StatusBadRequest, "server id is required")
		return
	}

	ctx := r.Context()

	row := h.db.QueryRow(ctx, "SELECT id, name, node_id, status, created_at, updated_at FROM servers WHERE id = ?", id)

	var server models.Server
	var createdAt, updatedAt string

	if err := row.Scan(&server.ID, &server.Name, &server.NodeID, &server.Status, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeServerError(w, http.StatusNotFound, fmt.Sprintf("server %q not found", id))
			return
		}
		log.Printf("get server scan error: %v", err)
		writeServerError(w, http.StatusInternalServerError, "failed to read server data")
		return
	}

	server.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	server.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	writeJSON(w, http.StatusOK, server)
}

func (h *ServerHandler) UpdateServer(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	// Validate Content-Type
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		writeServerError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	vars := mux.Vars(r)
	id := strings.TrimSpace(vars["id"])
	if id == "" {
		writeServerError(w, http.StatusBadRequest, "server id is required")
		return
	}

	var server models.Server
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&server); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxErr):
			writeServerError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON at byte %d", syntaxErr.Offset))
		case errors.As(err, &unmarshalErr):
			writeServerError(w, http.StatusBadRequest, fmt.Sprintf("invalid value for field %q", unmarshalErr.Field))
		case errors.Is(err, io.ErrUnexpectedEOF):
			writeServerError(w, http.StatusBadRequest, "truncated JSON body")
		default:
			writeServerError(w, http.StatusBadRequest, "invalid request body")
		}
		return
	}

	// Validate required fields
	if strings.TrimSpace(server.Name) == "" {
		writeServerError(w, http.StatusBadRequest, "name is required")
		return
	}
	if strings.TrimSpace(server.NodeID) == "" {
		writeServerError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if server.Status != "" && !isValidServerStatus(server.Status) {
		writeServerError(w, http.StatusBadRequest, fmt.Sprintf("invalid status %q", server.Status))
		return
	}

	server.UpdatedAt = time.Now().UTC()

	ctx := r.Context()

	result, err := h.db.Exec(ctx,
		"UPDATE servers SET name = ?, node_id = ?, status = ?, updated_at = ? WHERE id = ?",
		server.Name, server.NodeID, server.Status, server.UpdatedAt.Format(time.RFC3339), id)
	if err != nil {
		log.Printf("update server db error: %v", err)
		writeServerError(w, http.StatusInternalServerError, "failed to update server")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeServerError(w, http.StatusNotFound, fmt.Sprintf("server %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, server)
}

func (h *ServerHandler) DeleteServer(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	vars := mux.Vars(r)
	id := strings.TrimSpace(vars["id"])
	if id == "" {
		writeServerError(w, http.StatusBadRequest, "server id is required")
		return
	}

	ctx := r.Context()

	result, err := h.db.Exec(ctx, "DELETE FROM servers WHERE id = ?", id)
	if err != nil {
		log.Printf("delete server db error: %v", err)
		writeServerError(w, http.StatusInternalServerError, "failed to delete server")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		writeServerError(w, http.StatusNotFound, fmt.Sprintf("server %q not found", id))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

func isValidServerStatus(s models.ServerStatus) bool {
	switch s {
	case models.ServerStatusStarting, models.ServerStatusRunning,
		models.ServerStatusStopping, models.ServerStatusStopped,
		models.ServerStatusCrashed:
		return true
	}
	return false
}
