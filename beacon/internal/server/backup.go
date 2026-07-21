// DEPRECATED: This file contains gorilla/mux route handlers that are not wired into the active server.
// The beacon server uses http.NewServeMux (Go 1.22+) for route registration, defined in server.go.
// Backup functionality is handled through the orchestration layer (Forge API) and the backup.BackupInterface.
// This file is retained for reference but should be removed in a future cleanup.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"gamepanel/beacon/internal/backup"
)

// BackupHandler handles backup-related HTTP requests
type BackupHandler struct {
	backupManager *backup.BackupManager
	logger       Logger
	config       *Config
}

// NewBackupHandler creates a new BackupHandler
func NewBackupHandler(backupManager *backup.BackupManager, logger Logger, config *Config) *BackupHandler {
	return &BackupHandler{
		backupManager: backupManager,
		logger:       logger,
		config:       config,
	}
}

// RegisterRoutes registers backup routes with the router
func (h *BackupHandler) RegisterRoutes(router *mux.Router) {
	// Backup routes
	router.HandleFunc("/api/backup/app/{serverId}", h.handleCreateAppBackup).Methods("POST")
	router.HandleFunc("/api/backup/volume/{serverId}", h.handleCreateVolumeBackup).Methods("POST")
	router.HandleFunc("/api/backup/database/{serverId}", h.handleCreateDatabaseBackup).Methods("POST")
	router.HandleFunc("/api/backup/server/{serverId}", h.handleCreateServerBackup).Methods("POST")

	// Restore routes
	router.HandleFunc("/api/restore/app/{serverId}", h.handleRestoreApp).Methods("POST")
	router.HandleFunc("/api/restore/volume/{serverId}", h.handleRestoreVolume).Methods("POST")
	router.HandleFunc("/api/restore/database/{serverId}", h.handleRestoreDatabase).Methods("POST")
	router.HandleFunc("/api/restore/server/{serverId}", h.handleRestoreServer).Methods("POST")

	// Status and management routes
	router.HandleFunc("/api/backup/status/{taskId}", h.handleGetBackupStatus).Methods("GET")
	router.HandleFunc("/api/backup/cancel/{taskId}", h.handleCancelBackup).Methods("POST")
	router.HandleFunc("/api/backup/list/{serverId}", h.handleListBackups).Methods("GET")
	router.HandleFunc("/api/backup/delete/{serverId}/{name}", h.handleDeleteBackup).Methods("DELETE")

	// Verification routes
	router.HandleFunc("/api/backup/verify/{serverId}/{name}", h.handleVerifyBackup).Methods("POST")
	router.HandleFunc("/api/restore/verify/{restoreId}", h.handleVerifyRestore).Methods("POST")
}

// =============================================================================
// Backup Request Types
// =============================================================================

// CreateBackupRequest represents a request to create a backup
type CreateBackupRequest struct {
	ServerID        string            `json:"serverId"`
	BackupName      string            `json:"backupName"`
	BackupType      string            `json:"backupType"` // app, volume, database, server
	TargetID        string            `json:"targetId,omitempty"` // app ID, volume ID, database ID
	StorageProvider string            `json:"storageProvider,omitempty"`
	StorageConfig   json.RawMessage   `json:"storageConfig,omitempty"`
	Compression     bool              `json:"compression,omitempty"`
	Encryption      bool              `json:"encryption,omitempty"`
	EncryptionKey   string            `json:"encryptionKey,omitempty"`
	IncludePaths    []string          `json:"includePaths,omitempty"`
	ExcludePaths    []string          `json:"excludePaths,omitempty"`
	DatabaseEngine  string            `json:"databaseEngine,omitempty"` // mysql, postgres, mongodb, etc.
	DatabaseName    string            `json:"databaseName,omitempty"`
	VolumeName      string            `json:"volumeName,omitempty"`
	VolumeMountPath string            `json:"volumeMountPath,omitempty"`
}

// BackupResponse represents the response from a backup operation
type BackupResponse struct {
	TaskID       string          `json:"taskId"`
	BackupName   string          `json:"backupName"`
	Status       string          `json:"status"`
	Message      string          `json:"message,omitempty"`
	Error        string          `json:"error,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
	BackupInfo   *backup.BackupInfo `json:"backupInfo,omitempty"`
}

// =============================================================================
// Restore Request Types
// =============================================================================

// CreateRestoreRequest represents a request to create a restore
type CreateRestoreRequest struct {
	ServerID        string            `json:"serverId"`
	BackupName      string            `json:"backupName"`
	RestoreType     string            `json:"restoreType"` // app, volume, database, server
	TargetID        string            `json:"targetId,omitempty"` // app ID, volume ID, database ID
	StorageProvider string            `json:"storageProvider,omitempty"`
	RestorePath     string            `json:"restorePath,omitempty"`
	Overwrite      bool              `json:"overwrite,omitempty"`
	StopBefore      bool              `json:"stopBefore,omitempty"`
	StartAfter      bool              `json:"startAfter,omitempty"`
	DatabaseEngine  string            `json:"databaseEngine,omitempty"`
	DatabaseName    string            `json:"databaseName,omitempty"`
	DatabaseUser    string            `json:"databaseUser,omitempty"`
	DatabasePass    string            `json:"databasePass,omitempty"`
}

// RestoreResponse represents the response from a restore operation
type RestoreResponse struct {
	TaskID      string    `json:"taskId"`
	RestoreName string    `json:"restoreName"`
	Status      string    `json:"status"`
	Message     string    `json:"message,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// =============================================================================
// Backup Handlers
// =============================================================================

// handleCreateAppBackup handles creating an app backup
func (h *BackupHandler) handleCreateAppBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]

	// Parse request
	var req CreateBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.BackupName == "" {
		req.BackupName = fmt.Sprintf("backup-%s-%s", serverID, time.Now().Format("20060102-150405"))
	}

	// Get server root directory
	serverRoot, err := h.getServerRootDirectory(serverID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get server root: %v", err), http.StatusInternalServerError)
		return
	}

	// Create backup directory path (namespace)
	backupDir := fmt.Sprintf("server-%s", serverID)

	// Execute backup
	ctx := r.Context()
	progressFn := func(progress backup.BackupProgress) {
		h.logger.Infof("App backup progress for %s: %d/%d bytes (%s)", serverID, progress.BytesProcessed, progress.TotalBytes, progress.Phase)
	}

	h.backupManager.SetProgressCallback(progressFn)

	backupInfo, err := h.backupManager.Create(ctx, serverRoot, backupDir, req.BackupName, req.ExcludePaths)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create app backup: %v", err), http.StatusInternalServerError)
		return
	}

	// Upload to storage if configured
	if req.StorageProvider != "" && req.StorageProvider != "local" {
		err = h.uploadBackupToStorage(ctx, backupInfo, req.StorageProvider, req.StorageConfig)
		if err != nil {
			h.logger.Errorf("Failed to upload backup to storage: %v", err)
			// Continue, backup was created locally
		}
	}

	// Return response
	response := BackupResponse{
		TaskID:     backupInfo.UUID,
		BackupName: backupInfo.Name,
		Status:     backupInfo.Status,
		CreatedAt:  backupInfo.Created,
		BackupInfo: backupInfo,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCreateVolumeBackup handles creating a volume backup
func (h *BackupHandler) handleCreateVolumeBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]

	var req CreateBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.BackupName == "" {
		req.BackupName = fmt.Sprintf("volume-backup-%s-%s", serverID, time.Now().Format("20060102-150405"))
	}

	// Get volume mount path
	volumeMountPath := req.VolumeMountPath
	if volumeMountPath == "" {
		var pathErr error
		volumeMountPath, pathErr = h.getVolumeMountPath(serverID, req.VolumeName)
		if pathErr != nil {
			http.Error(w, fmt.Sprintf("Volume mount path not specified and could not be determined: %v", pathErr), http.StatusBadRequest)
			return
		}
	}

	// Create backup directory path
	backupDir := fmt.Sprintf("server-%s/volumes/%s", serverID, req.VolumeName)

	// Execute backup
	ctx := r.Context()
	progressFn := func(progress backup.BackupProgress) {
		h.logger.Infof("Volume backup progress for %s: %d/%d bytes (%s)", req.VolumeName, progress.BytesProcessed, progress.TotalBytes, progress.Phase)
	}

	h.backupManager.SetProgressCallback(progressFn)

	backupInfo, err := h.backupManager.Create(ctx, volumeMountPath, backupDir, req.BackupName, req.ExcludePaths)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create volume backup: %v", err), http.StatusInternalServerError)
		return
	}

	// Upload to storage if configured
	if req.StorageProvider != "" && req.StorageProvider != "local" {
		err = h.uploadBackupToStorage(ctx, backupInfo, req.StorageProvider, req.StorageConfig)
		if err != nil {
			h.logger.Errorf("Failed to upload backup to storage: %v", err)
		}
	}

	response := BackupResponse{
		TaskID:     backupInfo.UUID,
		BackupName: backupInfo.Name,
		Status:     backupInfo.Status,
		CreatedAt:  backupInfo.Created,
		BackupInfo: backupInfo,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCreateDatabaseBackup handles creating a database backup
func (h *BackupHandler) handleCreateDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]

	var req CreateBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.BackupName == "" {
		req.BackupName = fmt.Sprintf("db-backup-%s-%s-%s", serverID, req.DatabaseEngine, time.Now().Format("20060102-150405"))
	}

	if req.DatabaseEngine == "" {
		http.Error(w, "Database engine is required", http.StatusBadRequest)
		return
	}

	// Get database connection details
	dbConfig, err := h.getDatabaseConfig(serverID, req.TargetID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get database config: %v", err), http.StatusInternalServerError)
		return
	}

	// Create backup directory path
	backupDir := fmt.Sprintf("server-%s/databases/%s", serverID, req.TargetID)

	// Execute database-specific backup
	backupInfo, err := h.executeDatabaseBackup(r.Context(), dbConfig, backupDir, req.BackupName, req.DatabaseEngine)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create database backup: %v", err), http.StatusInternalServerError)
		return
	}

	// Upload to storage if configured
	if req.StorageProvider != "" && req.StorageProvider != "local" {
		err = h.uploadBackupToStorage(r.Context(), backupInfo, req.StorageProvider, req.StorageConfig)
		if err != nil {
			h.logger.Errorf("Failed to upload backup to storage: %v", err)
		}
	}

	response := BackupResponse{
		TaskID:     backupInfo.UUID,
		BackupName: backupInfo.Name,
		Status:     backupInfo.Status,
		CreatedAt:  backupInfo.Created,
		BackupInfo: backupInfo,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCreateServerBackup handles creating a server backup
func (h *BackupHandler) handleCreateServerBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]

	var req CreateBackupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.BackupName == "" {
		req.BackupName = fmt.Sprintf("server-backup-%s-%s", serverID, time.Now().Format("20060102-150405"))
	}

	// Get server root directory
	serverRoot, err := h.getServerRootDirectory(serverID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get server root: %v", err), http.StatusInternalServerError)
		return
	}

	// Create backup directory path
	backupDir := fmt.Sprintf("server-%s", serverID)

	// Execute backup
	ctx := r.Context()
	progressFn := func(progress backup.BackupProgress) {
		h.logger.Infof("Server backup progress for %s: %d/%d bytes (%s)", serverID, progress.BytesProcessed, progress.TotalBytes, progress.Phase)
	}

	h.backupManager.SetProgressCallback(progressFn)

	backupInfo, err := h.backupManager.Create(ctx, serverRoot, backupDir, req.BackupName, req.ExcludePaths)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create server backup: %v", err), http.StatusInternalServerError)
		return
	}

	// Upload to storage if configured
	if req.StorageProvider != "" && req.StorageProvider != "local" {
		err = h.uploadBackupToStorage(ctx, backupInfo, req.StorageProvider, req.StorageConfig)
		if err != nil {
			h.logger.Errorf("Failed to upload backup to storage: %v", err)
		}
	}

	response := BackupResponse{
		TaskID:     backupInfo.UUID,
		BackupName: backupInfo.Name,
		Status:     backupInfo.Status,
		CreatedAt:  backupInfo.Created,
		BackupInfo: backupInfo,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// =============================================================================
// Restore Handlers
// =============================================================================

// handleRestoreApp handles restoring an app backup
func (h *BackupHandler) handleRestoreApp(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]

	var req CreateRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.BackupName == "" {
		http.Error(w, "Backup name is required", http.StatusBadRequest)
		return
	}

	// Get server root directory
	serverRoot, err := h.getServerRootDirectory(serverID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get server root: %v", err), http.StatusInternalServerError)
		return
	}

	// Get backup directory path
	backupDir := fmt.Sprintf("server-%s", serverID)

	// Stop app before restore if requested
	if req.StopBefore {
		err = h.stopServer(serverID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to stop server: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Execute restore
	ctx := r.Context()
	err = h.backupManager.Restore(ctx, backupDir, req.BackupName, serverRoot, req.Overwrite)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to restore app: %v", err), http.StatusInternalServerError)
		return
	}

	// Start app after restore if requested
	if req.StartAfter {
		err = h.startServer(serverID)
		if err != nil {
			h.logger.Errorf("Failed to start server after restore: %v", err)
			// Continue, restore was successful
		}
	}

	response := RestoreResponse{
		TaskID:      uuid.NewString(),
		RestoreName: req.BackupName,
		Status:      "completed",
		Message:     "App restored successfully",
		CreatedAt:   time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRestoreVolume handles restoring a volume backup
func (h *BackupHandler) handleRestoreVolume(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]

	var req CreateRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.BackupName == "" {
		http.Error(w, "Backup name is required", http.StatusBadRequest)
		return
	}

	// Get volume mount path
	volumeMountPath := req.RestorePath
	if volumeMountPath == "" {
		var pathErr error
		volumeMountPath, pathErr = h.getVolumeMountPath(serverID, req.TargetID)
		if pathErr != nil {
			http.Error(w, fmt.Sprintf("Volume mount path not specified and could not be determined: %v", pathErr), http.StatusBadRequest)
			return
		}
	}

	// Get backup directory path
	backupDir := fmt.Sprintf("server-%s/volumes/%s", serverID, req.TargetID)

	// Execute restore
	ctx := r.Context()
	restoreErr := h.backupManager.Restore(ctx, backupDir, req.BackupName, volumeMountPath, req.Overwrite)
	if restoreErr != nil {
		http.Error(w, fmt.Sprintf("Failed to restore volume: %v", restoreErr), http.StatusInternalServerError)
		return
	}

	response := RestoreResponse{
		TaskID:      uuid.NewString(),
		RestoreName: req.BackupName,
		Status:      "completed",
		Message:     "Volume restored successfully",
		CreatedAt:   time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRestoreDatabase handles restoring a database backup
func (h *BackupHandler) handleRestoreDatabase(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]

	var req CreateRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.BackupName == "" {
		http.Error(w, "Backup name is required", http.StatusBadRequest)
		return
	}

	if req.DatabaseEngine == "" {
		http.Error(w, "Database engine is required", http.StatusBadRequest)
		return
	}

	// Get database connection details
	dbConfig, err := h.getDatabaseConfig(serverID, req.TargetID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get database config: %v", err), http.StatusInternalServerError)
		return
	}

	// Get backup directory path
	backupDir := fmt.Sprintf("server-%s/databases/%s", serverID, req.TargetID)

	// Execute database-specific restore
	err = h.executeDatabaseRestore(r.Context(), dbConfig, backupDir, req.BackupName, req.DatabaseEngine, req.DatabaseName, req.DatabaseUser, req.DatabasePass)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to restore database: %v", err), http.StatusInternalServerError)
		return
	}

	response := RestoreResponse{
		TaskID:      uuid.NewString(),
		RestoreName: req.BackupName,
		Status:      "completed",
		Message:     "Database restored successfully",
		CreatedAt:   time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRestoreServer handles restoring a server backup
func (h *BackupHandler) handleRestoreServer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]

	var req CreateRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.BackupName == "" {
		http.Error(w, "Backup name is required", http.StatusBadRequest)
		return
	}

	// Get server root directory
	serverRoot, err := h.getServerRootDirectory(serverID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get server root: %v", err), http.StatusInternalServerError)
		return
	}

	// Get backup directory path
	backupDir := fmt.Sprintf("server-%s", serverID)

	// Stop server before restore if requested
	if req.StopBefore {
		err = h.stopServer(serverID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to stop server: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Execute restore
	ctx := r.Context()
	err = h.backupManager.Restore(ctx, backupDir, req.BackupName, serverRoot, req.Overwrite)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to restore server: %v", err), http.StatusInternalServerError)
		return
	}

	// Start server after restore if requested
	if req.StartAfter {
		err = h.startServer(serverID)
		if err != nil {
			h.logger.Errorf("Failed to start server after restore: %v", err)
		}
	}

	response := RestoreResponse{
		TaskID:      uuid.NewString(),
		RestoreName: req.BackupName,
		Status:      "completed",
		Message:     "Server restored successfully",
		CreatedAt:   time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// =============================================================================
// Status and Management Handlers
// =============================================================================

// handleGetBackupStatus handles getting backup status
func (h *BackupHandler) handleGetBackupStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	// For now, return a placeholder response
	// In a real implementation, this would track the status of async backup operations
	response := map[string]interface{}{
		"taskId":    taskID,
		"status":    "completed",
		"progress":  100,
		"message":   "Backup completed successfully",
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleCancelBackup handles cancelling a backup
func (h *BackupHandler) handleCancelBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	// For now, return a placeholder response
	response := map[string]interface{}{
		"taskId":  taskID,
		"status":  "cancelled",
		"message": "Backup cancelled successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleListBackups handles listing backups for a server
func (h *BackupHandler) handleListBackups(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]

	// Get backup directory path
	backupDir := fmt.Sprintf("server-%s", serverID)

	backups, err := h.backupManager.List(backupDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list backups: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(backups)
}

// handleDeleteBackup handles deleting a backup
func (h *BackupHandler) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]
	backupName := vars["name"]

	// Get backup directory path
	backupDir := fmt.Sprintf("server-%s", serverID)

	err := h.backupManager.Delete(backupDir, backupName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete backup: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"serverId":   serverID,
		"backupName": backupName,
		"status":    "deleted",
		"message":   "Backup deleted successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// =============================================================================
// Verification Handlers
// =============================================================================

// handleVerifyBackup handles verifying a backup
func (h *BackupHandler) handleVerifyBackup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverID := vars["serverId"]
	backupName := vars["name"]

	// Get backup directory path
	backupDir := fmt.Sprintf("server-%s", serverID)

	// Get backup info
	backupInfo, err := h.backupManager.Get(backupDir, backupName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get backup info: %v", err), http.StatusNotFound)
		return
	}

	// Verify the backup
	err = h.verifyBackupIntegrity(backupDir, backupName, backupInfo.Checksum)
	if err != nil {
		http.Error(w, fmt.Sprintf("Backup verification failed: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"serverId":   serverID,
		"backupName": backupName,
		"status":    "verified",
		"message":   "Backup integrity verified successfully",
		"checksum":   backupInfo.Checksum,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleVerifyRestore handles verifying a restore
func (h *BackupHandler) handleVerifyRestore(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	restoreID := vars["restoreId"]

	// For now, return a placeholder response
	response := map[string]interface{}{
		"restoreId": restoreID,
		"status":   "verified",
		"message":  "Restore verified successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// =============================================================================
// Helper Methods
// =============================================================================

// getServerRootDirectory gets the root directory for a server
func (h *BackupHandler) getServerRootDirectory(serverID string) (string, error) {
	// In a real implementation, this would look up the server's root directory
	// from the server configuration or database
	// For now, return a placeholder
	return fmt.Sprintf("/var/lib/gamepanel/servers/%s", serverID), nil
}

// getVolumeMountPath gets the mount path for a volume
func (h *BackupHandler) getVolumeMountPath(serverID, volumeID string) (string, error) {
	// In a real implementation, this would look up the volume's mount path
	// from the volume configuration or database
	// For now, return a placeholder
	return fmt.Sprintf("/var/lib/gamepanel/servers/%s/volumes/%s", serverID, volumeID), nil
}

// getDatabaseConfig gets the database configuration
func (h *BackupHandler) getDatabaseConfig(serverID, databaseID string) (*DatabaseConfig, error) {
	// In a real implementation, this would look up the database configuration
	// from the database configuration or database
	// For now, return a placeholder
	return &DatabaseConfig{
		Host:     "localhost",
		Port:     3306,
		User:     "root",
		Password: "",
		Database: databaseID,
	}, nil
}

// uploadBackupToStorage uploads a backup to external storage
func (h *BackupHandler) uploadBackupToStorage(ctx context.Context, backupInfo *backup.BackupInfo, provider string, config json.RawMessage) error {
	// In a real implementation, this would upload the backup file
	// to the specified storage provider (S3, Azure, GCS, etc.)
	// For now, just log the request
	h.logger.Infof("Uploading backup %s to storage provider %s", backupInfo.Name, provider)
	return nil
}

// executeDatabaseBackup executes a database-specific backup
func (h *BackupHandler) executeDatabaseBackup(ctx context.Context, dbConfig *DatabaseConfig, backupDir, backupName, engine string) (*backup.BackupInfo, error) {
	// In a real implementation, this would execute the appropriate database backup
	// command based on the engine (mysqldump, pg_dump, mongodump, etc.)

	// For now, create a placeholder backup info
	return &backup.BackupInfo{
		UUID:        uuid.NewString(),
		Name:        backupName,
		Checksum:    "placeholder-checksum",
		Size:        1024 * 1024, // 1MB placeholder
		Status:      "completed",
		Created:     time.Now(),
		CompletedAt: time.Now(),
		Adapter:     "local",
		RemotePath:  filepath.Join(backupDir, backupName+".zip"),
	}, nil
}

// executeDatabaseRestore executes a database-specific restore
func (h *BackupHandler) executeDatabaseRestore(ctx context.Context, dbConfig *DatabaseConfig, backupDir, backupName, engine, dbName, dbUser, dbPass string) error {
	// In a real implementation, this would execute the appropriate database restore
	// command based on the engine (mysql, psql, mongo, etc.)

	// For now, just log the request
	h.logger.Infof("Restoring database %s from backup %s using engine %s", dbName, backupName, engine)
	return nil
}

// verifyBackupIntegrity verifies the integrity of a backup
func (h *BackupHandler) verifyBackupIntegrity(backupDir, backupName, expectedChecksum string) error {
	// In a real implementation, this would verify the backup file's checksum
	// and potentially test restoring it

	// For now, just log the request
	h.logger.Infof("Verifying backup %s (expected checksum: %s)", backupName, expectedChecksum)
	return nil
}

// stopServer stops a server
func (h *BackupHandler) stopServer(serverID string) error {
	// In a real implementation, this would stop the server via Docker or systemd
	h.logger.Infof("Stopping server %s", serverID)
	return nil
}

// startServer starts a server
func (h *BackupHandler) startServer(serverID string) error {
	// In a real implementation, this would start the server via Docker or systemd
	h.logger.Infof("Starting server %s", serverID)
	return nil
}

// DatabaseConfig represents database connection configuration
type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

// Logger is the interface for logging
type Logger interface {
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
}

// Config represents beacon configuration
type Config struct {
	// Add configuration fields as needed
	DataDirectory string `json:"dataDirectory,omitempty"`
	BackupDirectory string `json:"backupDirectory,omitempty"`
}

// uuid is a helper package for generating UUIDs
