package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gamepanel/beacon/internal/backup"
	"gamepanel/beacon/internal/ignore"
	"gamepanel/beacon/internal/remote"
	"gamepanel/beacon/internal/rootfs"
	"gamepanel/beacon/internal/tokens"
)

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	serverID := r.PathValue("id")
	root, err := s.safePath(serverID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := "backup-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".zip"

	// Read .pteroignore without following links outside the canonical server root.
	var ignored []string
	if fsys, fsErr := rootfs.New(root); fsErr == nil {
		if ignoreFile, openErr := fsys.Open(".pteroignore"); openErr == nil {
			if denylist, parseErr := ignore.LoadIgnoreReader(ignoreFile); parseErr == nil {
				ignored = denylist.Patterns()
			}
			_ = ignoreFile.Close()
		}
		_ = fsys.Close()
	}

	var reqBody struct {
		IgnoredFiles []string `json:"ignored_files"`
	}
	if r.Body != nil && r.Header.Get("Content-Type") == "application/json" {
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
	}
	if len(reqBody.IgnoredFiles) > 0 {
		ignored = append(ignored, reqBody.IgnoredFiles...)
	}

	if s.eventBus != nil {
		s.backups.SetProgressCallback(func(p backup.BackupProgress) {
			s.eventBus.Publish(BackupProgressEvent+":"+serverID, p)
		})
	}

	info, err := s.backups.Create(r.Context(), root, serverID, name, ignored)
	if err != nil {
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}

	if s.panelClient != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.panelClient.SendBackupStatus(ctx, serverID, remote.BackupStatusRequest{
				BackupUUID: info.UUID,
				ServerUUID: serverID,
				Checksum:   info.Checksum,
				Size:       info.Size,
				Successful: true,
			})
		}()
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"uuid":         info.UUID,
		"name":         info.Name,
		"checksum":     info.Checksum,
		"size":         info.Size,
		"status":       info.Status,
		"created":      info.Created.Format(time.RFC3339),
		"completedAt":  info.CompletedAt.Format(time.RFC3339),
		"adapter":      info.Adapter,
		"ignoredFiles": info.IgnoredFiles,
	})
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	serverID := r.PathValue("id")
	if _, err := s.safePath(serverID, ""); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	backups, err := s.backups.List(serverID)
	if err != nil {
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, backups)
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	name := r.URL.Query().Get("name")
	if !safeBackupName(name) {
		http.Error(w, "invalid backup name", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	if _, err := s.safePath(serverID, ""); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reader, err := s.backups.Download(serverID, name)
	if err != nil {
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = io.Copy(w, reader)
}

func (s *Server) downloadBackupWithToken(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	if s.tokenGenerator == nil {
		http.Error(w, "token generator not configured", http.StatusInternalServerError)
		return
	}
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}
	claims, err := s.tokenGenerator.Validate(tokenStr)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}
	if claims.Scope != tokens.ScopeBackupDownload {
		http.Error(w, "invalid token scope", http.StatusForbidden)
		return
	}
	serverID := claims.ServerID
	backupUUID := claims.BackupID
	if backupUUID == "" {
		http.Error(w, "invalid token: missing backup id", http.StatusBadRequest)
		return
	}
	name := backupUUID + ".zip"
	if !safeBackupName(name) {
		http.Error(w, "invalid backup name", http.StatusBadRequest)
		return
	}
	if _, err := s.safePath(serverID, ""); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reader, err := s.backups.Download(serverID, name)
	if err != nil {
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = io.Copy(w, reader)
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Name     string `json:"name"`
		Truncate bool   `json:"truncate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if !safeBackupName(body.Name) {
		http.Error(w, "invalid backup name", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	root, err := s.safePath(serverID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.backups.Restore(r.Context(), serverID, body.Name, root, body.Truncate); err != nil {
		if s.panelClient != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = s.panelClient.SendRestoreStatus(ctx, serverID, remote.RestoreStatusRequest{
					BackupUUID: strings.TrimSuffix(body.Name, ".zip"),
					ServerUUID: serverID,
					Successful: false,
					Error:      err.Error(),
				})
			}()
		}
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}

	if s.panelClient != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.panelClient.SendRestoreStatus(ctx, serverID, remote.RestoreStatusRequest{
				BackupUUID: strings.TrimSuffix(body.Name, ".zip"),
				ServerUUID: serverID,
				Successful: true,
			})
		}()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": body.Name, "status": "restored"})
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	// Support both path-value (/backups/{backupId}) and legacy query-param (?name=).
	name := r.PathValue("backupId")
	if name == "" {
		name = r.URL.Query().Get("name")
	}
	if !safeBackupName(name) {
		http.Error(w, "invalid backup name", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	if _, err := s.safePath(serverID, ""); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.backups.Delete(serverID, name); err != nil {
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) backupProgressWS(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	defer s.trackWebSocket(r, conn)()
	configureWebSocket(conn)
	writer := &webSocketWriter{conn: conn}
	done := make(chan struct{})
	defer close(done)
	go pingWebSocket(writer, done)

	serverID := r.PathValue("id")
	ch := s.eventBus.Subscribe(BackupProgressEvent + ":" + serverID)
	defer s.eventBus.Unsubscribe(BackupProgressEvent+":"+serverID, ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := writer.Write(msg); err != nil {
				return
			}
		}
	}
}

func backupErrorStatus(err error) int {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, backup.ErrInvalidName), errors.Is(err, backup.ErrInvalidNamespace):
		return http.StatusBadRequest
	case errors.Is(err, backup.ErrChecksumMismatch):
		return http.StatusUnprocessableEntity
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusRequestTimeout
	default:
		return http.StatusInternalServerError
	}
}

func safeBackupName(name string) bool {
	if name == "" || len(name) > 128 || !strings.HasSuffix(name, ".zip") || strings.Contains(name, "..") {
		return false
	}
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(buf)
}
