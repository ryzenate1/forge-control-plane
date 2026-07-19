package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"

	"gamepanel/beacon/internal/serverid"
)

func (s *Server) startTransfer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetNode string `json:"targetNode"`
		TargetURL  string `json:"targetUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.TargetNode == "" || body.TargetURL == "" {
		http.Error(w, "targetNode and targetUrl are required", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	serverRoot, err := s.safePath(serverID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	transferMgr := getTransferManager()
	transfer, err := transferMgr.Start(r.Context(), serverID, "local", body.TargetNode, serverRoot, body.TargetURL, s.token, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, transfer)
}

func (s *Server) getTransferStatus(w http.ResponseWriter, r *http.Request) {
	transferID := r.PathValue("transferId")
	transferMgr := getTransferManager()
	transfer, ok := transferMgr.Get(transferID)
	if !ok {
		http.Error(w, "transfer not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, transfer)
}

func (s *Server) cancelTransfer(w http.ResponseWriter, r *http.Request) {
	transferID := r.PathValue("transferId")
	transferMgr := getTransferManager()
	if err := transferMgr.Cancel(transferID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// receiveTransferArchive is the destination-side endpoint for incoming
// server-to-server transfers. The source daemon POSTs a multipart payload
// containing:
//   - "archive" part: tar.gz archive stream of the server files
//   - "checksum" part: hex-encoded SHA256 of the archive
//
// The handler:
//  1. Verifies the SHA256 checksum matches the archive stream
//  2. Extracts the archive to the destination server's root directory
//  3. Notifies the source daemon that the transfer is complete
func (s *Server) receiveTransferArchive(w http.ResponseWriter, r *http.Request) {
	serverID := r.Header.Get("X-Transfer-ServerID")
	if err := serverid.Validate(serverID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	transferID := r.Header.Get("X-Transfer-ID")
	if !safeUploadID(transferID) {
		http.Error(w, "invalid transfer id", http.StatusBadRequest)
		return
	}
	resumeOffset := strings.TrimSpace(r.Header.Get("X-Transfer-Resume-Offset"))
	if resumeOffset != "" {
		offset, err := strconv.ParseInt(resumeOffset, 10, 64)
		if err != nil || offset < 0 {
			http.Error(w, "invalid transfer resume offset", http.StatusBadRequest)
			return
		}
		if offset != 0 {
			http.Error(w, "transfer resume is not supported by the destination", http.StatusNotImplemented)
			return
		}
	}
	expectedChecksum := r.Header.Get("X-Checksum")
	if expectedChecksum == "" {
		http.Error(w, "X-Checksum header is required", http.StatusBadRequest)
		return
	}

	fsys, err := s.serverFilesystem(serverID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	if err := fsys.MkdirAll(".backups", 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tempName := path.Join(".backups", ".transfer-"+transferID+".tar.gz")
	out, err := fsys.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hasher := sha256.New()
	const transferLimit = int64(32 * 1024 * 1024 * 1024)
	written, copyErr := io.Copy(io.MultiWriter(out, hasher), io.LimitReader(r.Body, transferLimit+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || written > transferLimit {
		_ = fsys.RemoveAll(tempName)
		if written > transferLimit {
			http.Error(w, "transfer archive too large", http.StatusRequestEntityTooLarge)
		} else if copyErr != nil {
			http.Error(w, copyErr.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, closeErr.Error(), http.StatusInternalServerError)
		}
		return
	}
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if actualChecksum != expectedChecksum {
		_ = fsys.RemoveAll(tempName)
		http.Error(w, fmt.Sprintf("checksum mismatch (expected=%s actual=%s)", expectedChecksum, actualChecksum), http.StatusBadRequest)
		return
	}
	if _, err := extractArchive(fsys, tempName, "", s.manager, serverID); err != nil {
		_ = fsys.RemoveAll(tempName)
		http.Error(w, fmt.Sprintf("extract failed: %v", err), http.StatusBadRequest)
		return
	}
	_ = fsys.RemoveAll(tempName)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"serverId":   serverID,
		"transferId": transferID,
		"bytes":      written,
		"checksum":   actualChecksum,
	})
}
