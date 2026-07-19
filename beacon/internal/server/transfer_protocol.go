package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gamepanel/beacon/internal/transfer"
)

type sourcePushRequest struct {
	DestinationURL        string `json:"destinationUrl"`
	DestinationCredential string `json:"destinationCredential"`
	IdempotencyKey        string `json:"idempotencyKey"`
}

func (s *Server) registerTransferCredential(w http.ResponseWriter, r *http.Request) {
	if s.transferProtocol == nil {
		http.Error(w, "transfer protocol unavailable", http.StatusServiceUnavailable)
		return
	}
	var registration transfer.CredentialRegistration
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&registration); err != nil {
		http.Error(w, "invalid credential registration", http.StatusBadRequest)
		return
	}
	if err := s.transferProtocol.Register(registration); err != nil {
		writeTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"version": transfer.ProtocolVersion, "registered": true})
}

func (s *Server) prepareTransferSource(w http.ResponseWriter, r *http.Request) {
	credential, ok := transferBearer(w, r)
	if !ok || s.transferProtocol == nil {
		return
	}
	migrationID := r.PathValue("id")
	claims, err := s.transferProtocol.Authorize(migrationID, transfer.DirectionSourceControl, credential)
	if err != nil {
		writeTransferError(w, err)
		return
	}
	if claims.ServerID == "" {
		http.Error(w, "invalid server binding", http.StatusBadRequest)
		return
	}
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	actual, err := s.runtime.Inspect(r.Context(), claims.ServerID)
	if err != nil {
		http.Error(w, "inspect source container: "+err.Error(), http.StatusConflict)
		return
	}
	if actual.Exists && actual.Running {
		if err := s.manager.HandlePower(r.Context(), claims.ServerID, "stop"); err != nil {
			http.Error(w, "stop source server: "+err.Error(), http.StatusConflict)
			return
		}
	}
	meta, err := s.transferProtocol.PrepareSource(r.Context(), migrationID, credential)
	if err != nil {
		writeTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) pushTransferSource(w http.ResponseWriter, r *http.Request) {
	credential, ok := transferBearer(w, r)
	if !ok || s.transferProtocol == nil {
		return
	}
	var body sourcePushRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&body); err != nil || body.DestinationURL == "" || body.DestinationCredential == "" {
		http.Error(w, "destinationUrl and destinationCredential are required", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(body.DestinationURL, "https://") && !strings.HasPrefix(body.DestinationURL, "http://") {
		http.Error(w, "invalid destinationUrl", http.StatusBadRequest)
		return
	}
	meta, err := s.pushArchive(r.Context(), r.PathValue("id"), credential, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) pushArchive(ctx context.Context, migrationID, sourceCredential string, request sourcePushRequest) (transfer.Metadata, error) {
	endpoint := strings.TrimRight(request.DestinationURL, "/") + "/api/v1/transfers/" + migrationID + "/destination/archive"
	client := &http.Client{Timeout: 0}
	for attempts := 0; attempts < 4; attempts++ {
		head, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
		if err != nil {
			return transfer.Metadata{}, err
		}
		head.Header.Set("Authorization", "Bearer "+request.DestinationCredential)
		response, err := client.Do(head)
		if err != nil {
			return transfer.Metadata{}, err
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			message := responseMessage(response)
			return transfer.Metadata{}, fmt.Errorf("destination offset negotiation failed: %s", message)
		}
		offset, err := strconv.ParseInt(response.Header.Get("Upload-Offset"), 10, 64)
		_ = response.Body.Close()
		if err != nil || offset < 0 {
			return transfer.Metadata{}, errors.New("destination returned invalid upload offset")
		}
		archive, source, err := s.transferProtocol.SourceArchive(migrationID, sourceCredential, offset)
		if err != nil {
			return transfer.Metadata{}, err
		}
		patch, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, archive)
		if err != nil {
			_ = archive.Close()
			return transfer.Metadata{}, err
		}
		patch.Header.Set("Authorization", "Bearer "+request.DestinationCredential)
		patch.Header.Set("Content-Type", "application/offset+octet-stream")
		patch.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
		patch.Header.Set("Upload-Length", strconv.FormatInt(source.ArchiveSize, 10))
		patch.Header.Set("Upload-Checksum", "sha256 "+source.Checksum)
		patch.Header.Set("Idempotency-Key", request.IdempotencyKey)
		patch.ContentLength = source.ArchiveSize - offset
		response, err = client.Do(patch)
		_ = archive.Close()
		if err != nil {
			if ctx.Err() != nil {
				return transfer.Metadata{}, ctx.Err()
			}
			continue
		}
		if response.StatusCode == http.StatusConflict {
			_ = response.Body.Close()
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return transfer.Metadata{}, fmt.Errorf("destination upload failed: %s", responseMessage(response))
		}
		var destination transfer.Metadata
		err = json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&destination)
		_ = response.Body.Close()
		if err != nil {
			return transfer.Metadata{}, err
		}
		if destination.Phase != "verified" {
			return destination, errors.New("destination did not verify complete archive")
		}
		return destination, nil
	}
	return transfer.Metadata{}, errors.New("destination offset remained inconsistent after retries")
}

func (s *Server) sourceTransferStatus(w http.ResponseWriter, r *http.Request) {
	credential, ok := transferBearer(w, r)
	if !ok || s.transferProtocol == nil {
		return
	}
	meta, err := s.transferProtocol.Status(r.PathValue("id"), transfer.DirectionSourceControl, credential)
	if err != nil {
		writeTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) cleanupTransferSource(w http.ResponseWriter, r *http.Request) {
	credential, ok := transferBearer(w, r)
	if !ok || s.transferProtocol == nil {
		return
	}
	meta, err := s.transferProtocol.Authorize(r.PathValue("id"), transfer.DirectionSourceControl, credential)
	if err != nil {
		writeTransferError(w, err)
		return
	}
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	if err := s.runtime.Delete(r.Context(), meta.ServerID); err != nil && !isContainerMissing(err) {
		http.Error(w, "delete source container: "+err.Error(), http.StatusConflict)
		return
	}
	if err := s.transferProtocol.CleanupSource(r.PathValue("id"), credential); err != nil {
		writeTransferError(w, err)
		return
	}
	s.manager.Delete(meta.ServerID)
	writeJSON(w, http.StatusOK, map[string]any{"cleaned": true})
}

func (s *Server) destinationTransferOffset(w http.ResponseWriter, r *http.Request) {
	credential, ok := transferBearer(w, r)
	if !ok || s.transferProtocol == nil {
		return
	}
	meta, err := s.transferProtocol.DestinationOffset(r.PathValue("id"), credential)
	if err != nil {
		writeTransferError(w, err)
		return
	}
	w.Header().Set("Transfer-Protocol", transfer.ProtocolVersion)
	w.Header().Set("Upload-Offset", strconv.FormatInt(meta.Offset, 10))
	if meta.ArchiveSize > 0 {
		w.Header().Set("Upload-Length", strconv.FormatInt(meta.ArchiveSize, 10))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) receiveTransferChunk(w http.ResponseWriter, r *http.Request) {
	credential, ok := transferBearer(w, r)
	if !ok || s.transferProtocol == nil {
		return
	}
	offset, offsetErr := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	total, totalErr := strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
	checksum := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Upload-Checksum"), "sha256 "))
	if offsetErr != nil || totalErr != nil || checksum == "" {
		http.Error(w, "invalid upload metadata", http.StatusBadRequest)
		return
	}
	meta, err := s.transferProtocol.AppendDestination(r.Context(), r.PathValue("id"), credential, offset, total, checksum, r.Body)
	if err != nil {
		writeTransferError(w, err)
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(meta.Offset, 10))
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) restoreTransferDestination(w http.ResponseWriter, r *http.Request) {
	credential, ok := transferBearer(w, r)
	if !ok || s.transferProtocol == nil {
		return
	}
	meta, err := s.transferProtocol.RestoreDestination(r.Context(), r.PathValue("id"), credential)
	if err != nil {
		writeTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) finalizeTransferDestination(w http.ResponseWriter, r *http.Request) {
	credential, ok := transferBearer(w, r)
	if !ok || s.transferProtocol == nil {
		return
	}
	if err := s.transferProtocol.FinalizeDestination(r.PathValue("id"), credential); err != nil {
		writeTransferError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"activated": true})
}

func (s *Server) cancelProtocolTransfer(w http.ResponseWriter, r *http.Request) {
	credential, ok := transferBearer(w, r)
	if !ok || s.transferProtocol == nil {
		return
	}
	migrationID := r.PathValue("id")
	if _, err := s.transferProtocol.Authorize(migrationID, transfer.DirectionSourceControl, credential); err != nil {
		if _, uploadErr := s.transferProtocol.Authorize(migrationID, transfer.DirectionDestinationUpload, credential); uploadErr != nil {
			writeTransferError(w, err)
			return
		}
	}
	_ = s.transferProtocol.Cancel(migrationID)
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
}

func transferBearer(w http.ResponseWriter, r *http.Request) (string, bool) {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(value, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(value, "Bearer ")) == "" {
		http.Error(w, "transfer bearer credential required", http.StatusUnauthorized)
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(value, "Bearer ")), true
}

func writeTransferError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, transfer.ErrUnauthorized), errors.Is(err, transfer.ErrExpired), errors.Is(err, transfer.ErrReplayed):
		status = http.StatusUnauthorized
	case errors.Is(err, transfer.ErrOffsetMismatch), errors.Is(err, transfer.ErrChecksumMismatch), errors.Is(err, transfer.ErrInvalidBinding):
		status = http.StatusConflict
	case errors.Is(err, context.Canceled):
		status = http.StatusRequestTimeout
	}
	http.Error(w, err.Error(), status)
}

func responseMessage(response *http.Response) string {
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 16*1024))
	message := strings.TrimSpace(string(bytes.TrimSpace(body)))
	if message == "" {
		message = response.Status
	}
	return message
}

var _ = time.Second
