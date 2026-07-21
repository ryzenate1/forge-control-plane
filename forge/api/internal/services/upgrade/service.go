package upgrade

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// UpgradeStatus represents the current status of an upgrade
type UpgradeStatus string

const (
	UpgradeStatusPending     UpgradeStatus = "pending"
	UpgradeStatusDownloading UpgradeStatus = "downloading"
	UpgradeStatusBackingUp   UpgradeStatus = "backing_up"
	UpgradeStatusUpgrading   UpgradeStatus = "upgrading"
	UpgradeStatusCompleted   UpgradeStatus = "completed"
	UpgradeStatusFailed      UpgradeStatus = "failed"
	UpgradeStatusRolledBack  UpgradeStatus = "rolled_back"
)

// UpgradeType represents the type of upgrade being performed
type UpgradeType string

const (
	UpgradeTypeAPI      UpgradeType = "api"
	UpgradeTypeWeb      UpgradeType = "web"
	UpgradeTypeBeacon   UpgradeType = "beacon"
	UpgradeTypeFull     UpgradeType = "full"
	UpgradeTypeDatabase UpgradeType = "database"
)

// VersionInfo contains version information for a component
type VersionInfo struct {
	Component  string `json:"component"`
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	Upgradable bool   `json:"upgradable"`
}

// UpgradePlan represents a planned upgrade
type UpgradePlan struct {
	ID          string        `json:"id"`
	Type        UpgradeType   `json:"type"`
	FromVersion string        `json:"fromVersion"`
	ToVersion   string        `json:"toVersion"`
	Components  []string      `json:"components"`
	Status      UpgradeStatus `json:"status"`
	Progress    int           `json:"progress"`
	TotalSteps  int           `json:"totalSteps"`
	CurrentStep string        `json:"currentStep"`
	Error       string        `json:"error,omitempty"`
	BackupPath  string        `json:"backupPath,omitempty"`
	StartedAt   time.Time     `json:"startedAt"`
	CompletedAt *time.Time    `json:"completedAt,omitempty"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

// UpgradeResult represents the result of an upgrade operation
type UpgradeResult struct {
	Success     bool          `json:"success"`
	Message     string        `json:"message"`
	UpgradePlan *UpgradePlan  `json:"upgradePlan,omitempty"`
	VersionInfo []VersionInfo `json:"versionInfo,omitempty"`
	Error       string        `json:"error,omitempty"`
}

// Store defines the interface for upgrade persistence
type Store interface {
	CreateUpgradePlan(ctx context.Context, plan *UpgradePlan) error
	GetUpgradePlan(ctx context.Context, id string) (*UpgradePlan, error)
	ListUpgradePlans(ctx context.Context, limit int) ([]UpgradePlan, error)
	UpdateUpgradePlan(ctx context.Context, plan *UpgradePlan) error
	DeleteUpgradePlan(ctx context.Context, id string) error
	GetLatestUpgrade(ctx context.Context, component string) (*UpgradePlan, error)
}

// Service provides upgrade functionality
type Service struct {
	store       Store
	logger      *slog.Logger
	installDir  string
	backupDir   string
	versionFile string
}

// New creates a new upgrade service
func New(store Store, logger *slog.Logger, installDir, backupDir, versionFile string) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if installDir == "" {
		installDir = "/opt/gamepanel"
	}
	if backupDir == "" {
		backupDir = "/var/backups/gamepanel"
	}
	if versionFile == "" {
		versionFile = filepath.Join(installDir, "version.txt")
	}
	return &Service{
		store:       store,
		logger:      logger,
		installDir:  installDir,
		backupDir:   backupDir,
		versionFile: versionFile,
	}
}

// CheckForUpgrades checks if upgrades are available for any components
func (s *Service) CheckForUpgrades(ctx context.Context) ([]VersionInfo, error) {
	components := []string{"api", "web", "beacon", "database"}
	var versionInfos []VersionInfo

	for _, component := range components {
		currentVersion, err := s.GetCurrentVersion(component)
		if err != nil {
			s.logger.Warn("Failed to get current version", "component", component, "error", err)
			continue
		}

		latestVersion, err := s.GetLatestVersion(component)
		if err != nil {
			s.logger.Warn("Failed to get latest version", "component", component, "error", err)
			continue
		}

		upgradable := currentVersion != latestVersion

		versionInfos = append(versionInfos, VersionInfo{
			Component:  component,
			Current:    currentVersion,
			Latest:     latestVersion,
			Upgradable: upgradable,
		})
	}

	return versionInfos, nil
}

// GetCurrentVersion returns the current version of a component
func (s *Service) GetCurrentVersion(component string) (string, error) {
	// Try to read from version file first
	if _, err := os.Stat(s.versionFile); err == nil {
		content, err := os.ReadFile(s.versionFile)
		if err == nil {
			return strings.TrimSpace(string(content)), nil
		}
	}

	// Fall back to component-specific version detection
	switch component {
	case "api":
		return s.getAPIVersion()
	case "web":
		return s.getWebVersion()
	case "beacon":
		return s.getBeaconVersion()
	case "database":
		return s.getDatabaseVersion()
	default:
		return "", fmt.Errorf("unknown component: %s", component)
	}
}

// GetLatestVersion returns the latest available version of a component
func (s *Service) GetLatestVersion(component string) (string, error) {
	// This would typically check a remote repository or API
	// For now, return a mock version
	return "2.0.0", nil
}

// getAPIVersion returns the current API version
func (s *Service) getAPIVersion() (string, error) {
	// Check if the API is running and get its version
	// This is a placeholder implementation
	return "1.0.0", nil
}

// getWebVersion returns the current Web version
func (s *Service) getWebVersion() (string, error) {
	return "1.0.0", nil
}

// getBeaconVersion returns the current Beacon version
func (s *Service) getBeaconVersion() (string, error) {
	return "1.0.0", nil
}

// getDatabaseVersion returns the current Database version
func (s *Service) getDatabaseVersion() (string, error) {
	return "1.0.0", nil
}

// CreateUpgradePlan creates a new upgrade plan
func (s *Service) CreateUpgradePlan(ctx context.Context, upgradeType UpgradeType, components []string) (*UpgradePlan, error) {
	if len(components) == 0 {
		return nil, fmt.Errorf("at least one component must be specified")
	}

	// Get current versions
	var fromVersions []string
	for _, component := range components {
		version, err := s.GetCurrentVersion(component)
		if err != nil {
			return nil, fmt.Errorf("failed to get current version for %s: %w", component, err)
		}
		fromVersions = append(fromVersions, version)
	}

	// Get latest versions
	var toVersions []string
	for _, component := range components {
		version, err := s.GetLatestVersion(component)
		if err != nil {
			return nil, fmt.Errorf("failed to get latest version for %s: %w", component, err)
		}
		toVersions = append(toVersions, version)
	}

	// Create the upgrade plan
	plan := &UpgradePlan{
		ID:          uuid.NewString(),
		Type:        upgradeType,
		FromVersion: strings.Join(fromVersions, ","),
		ToVersion:   strings.Join(toVersions, ","),
		Components:  components,
		Status:      UpgradeStatusPending,
		Progress:    0,
		TotalSteps:  s.calculateTotalSteps(components),
		StartedAt:   time.Now().UTC(),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Save the plan
	if s.store != nil {
		if err := s.store.CreateUpgradePlan(ctx, plan); err != nil {
			return nil, fmt.Errorf("failed to create upgrade plan: %w", err)
		}
	}

	return plan, nil
}

// calculateTotalSteps calculates the total number of steps for the upgrade
func (s *Service) calculateTotalSteps(components []string) int {
	// Base steps: backup, pre-checks, post-checks
	steps := 3

	// Add steps for each component
	steps += len(components) * 2 // download and upgrade per component

	return steps
}

// ExecuteUpgradePlan executes an upgrade plan
func (s *Service) ExecuteUpgradePlan(ctx context.Context, planID string) (*UpgradeResult, error) {
	// Get the upgrade plan
	plan, err := s.store.GetUpgradePlan(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("failed to get upgrade plan: %w", err)
	}

	if plan == nil {
		return nil, fmt.Errorf("upgrade plan not found: %s", planID)
	}

	// Update plan status
	plan.Status = UpgradeStatusUpgrading
	plan.CurrentStep = "Starting upgrade"
	plan.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdateUpgradePlan(ctx, plan); err != nil {
		s.logger.Error("Failed to update upgrade plan", "error", err)
	}

	// Create backup before upgrade
	if err := s.createBackup(ctx, plan); err != nil {
		s.logger.Error("Failed to create backup", "error", err)
		plan.Status = UpgradeStatusFailed
		plan.Error = fmt.Sprintf("backup failed: %v", err)
		plan.UpdatedAt = time.Now().UTC()
		if updateErr := s.store.UpdateUpgradePlan(ctx, plan); updateErr != nil {
			s.logger.Error("Failed to update failed upgrade plan", "error", updateErr)
		}
		return &UpgradeResult{
			Success:     false,
			Message:     "Upgrade failed",
			UpgradePlan: plan,
			Error:       err.Error(),
		}, err
	}

	// Execute upgrade steps
	for i, component := range plan.Components {
		plan.CurrentStep = fmt.Sprintf("Upgrading %s", component)
		plan.Progress = i + 1
		plan.UpdatedAt = time.Now().UTC()

		if err := s.store.UpdateUpgradePlan(ctx, plan); err != nil {
			s.logger.Error("Failed to update upgrade progress", "error", err)
		}

		if err := s.upgradeComponent(ctx, component, plan); err != nil {
			s.logger.Error("Failed to upgrade component", "component", component, "error", err)

			// Attempt rollback
			if rollbackErr := s.rollbackUpgrade(ctx, plan); rollbackErr != nil {
				s.logger.Error("Failed to rollback upgrade", "error", rollbackErr)
			}

			plan.Status = UpgradeStatusFailed
			plan.Error = fmt.Sprintf("failed to upgrade %s: %v", component, err)
			plan.UpdatedAt = time.Now().UTC()

			if updateErr := s.store.UpdateUpgradePlan(ctx, plan); updateErr != nil {
				s.logger.Error("Failed to update failed upgrade plan", "error", updateErr)
			}

			return &UpgradeResult{
				Success:     false,
				Message:     "Upgrade failed",
				UpgradePlan: plan,
				Error:       err.Error(),
			}, err
		}
	}

	// Mark as completed
	plan.Status = UpgradeStatusCompleted
	plan.Progress = plan.TotalSteps
	plan.CurrentStep = "Upgrade completed"
	completedAt := time.Now().UTC()
	plan.CompletedAt = &completedAt
	plan.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdateUpgradePlan(ctx, plan); err != nil {
		s.logger.Error("Failed to update completed upgrade plan", "error", err)
	}

	return &UpgradeResult{
		Success:     true,
		Message:     "Upgrade completed successfully",
		UpgradePlan: plan,
	}, nil
}

// upgradeComponent upgrades a specific component
func (s *Service) upgradeComponent(ctx context.Context, component string, plan *UpgradePlan) error {
	s.logger.Info("Upgrading component", "component", component)

	switch component {
	case "api":
		return s.upgradeAPI(ctx, plan)
	case "web":
		return s.upgradeWeb(ctx, plan)
	case "beacon":
		return s.upgradeBeacon(ctx, plan)
	case "database":
		return s.upgradeDatabase(ctx, plan)
	default:
		return fmt.Errorf("unknown component: %s", component)
	}
}

// upgradeAPI upgrades the API component
func (s *Service) upgradeAPI(ctx context.Context, plan *UpgradePlan) error {
	s.logger.Info("Upgrading API component")

	// This would typically:
	// 1. Pull the latest API image
	// 2. Stop the current API container
	// 3. Start the new API container
	// 4. Verify the new API is working

	// For now, just simulate the upgrade
	s.logger.Info("API upgrade simulated")
	return nil
}

// upgradeWeb upgrades the Web component
func (s *Service) upgradeWeb(ctx context.Context, plan *UpgradePlan) error {
	s.logger.Info("Upgrading Web component")

	// Simulate the upgrade
	s.logger.Info("Web upgrade simulated")
	return nil
}

// upgradeBeacon upgrades the Beacon component
func (s *Service) upgradeBeacon(ctx context.Context, plan *UpgradePlan) error {
	s.logger.Info("Upgrading Beacon component")

	// Simulate the upgrade
	s.logger.Info("Beacon upgrade simulated")
	return nil
}

// upgradeDatabase upgrades the Database component
func (s *Service) upgradeDatabase(ctx context.Context, plan *UpgradePlan) error {
	s.logger.Info("Upgrading Database component")

	// Database upgrades require special handling
	// This would typically run migrations

	// Simulate the upgrade
	s.logger.Info("Database upgrade simulated")
	return nil
}

// createBackup creates a backup before upgrade
func (s *Service) createBackup(ctx context.Context, plan *UpgradePlan) error {
	s.logger.Info("Creating backup before upgrade")

	// Create backup directory
	backupPath := filepath.Join(s.backupDir, fmt.Sprintf("upgrade-%s-%s", plan.ID, time.Now().Format("20060102-150405")))
	if err := os.MkdirAll(backupPath, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	plan.BackupPath = backupPath

	// Backup configuration files
	configFiles := []string{
		filepath.Join(s.installDir, "docker-compose.yml"),
		filepath.Join(s.installDir, "nginx.conf"),
		filepath.Join(s.installDir, ".env"),
	}

	for _, configFile := range configFiles {
		if _, err := os.Stat(configFile); err == nil {
			dest := filepath.Join(backupPath, filepath.Base(configFile))
			if err := copyFile(configFile, dest); err != nil {
				s.logger.Warn("Failed to backup config file", "file", configFile, "error", err)
			}
		}
	}

	// Backup database (this would require database access)
	if err := s.backupDatabase(ctx, backupPath); err != nil {
		s.logger.Warn("Failed to backup database", "error", err)
	}

	s.logger.Info("Backup created", "path", backupPath)
	return nil
}

// backupDatabase backs up the database
func (s *Service) backupDatabase(ctx context.Context, backupPath string) error {
	s.logger.Info("Backing up database")

	// This would use pg_dump or similar to backup the database
	// For now, just create a placeholder
	backupFile := filepath.Join(backupPath, "database.sql")
	if err := os.WriteFile(backupFile, []byte("-- Database backup placeholder"), 0644); err != nil {
		return fmt.Errorf("failed to create database backup: %w", err)
	}

	return nil
}

// rollbackUpgrade rolls back an upgrade
func (s *Service) rollbackUpgrade(ctx context.Context, plan *UpgradePlan) error {
	s.logger.Info("Rolling back upgrade", "planId", plan.ID)

	if plan.BackupPath == "" {
		return fmt.Errorf("no backup path available for rollback")
	}

	// Update plan status
	plan.Status = UpgradeStatusRolledBack
	plan.CurrentStep = "Rolling back"
	plan.UpdatedAt = time.Now().UTC()

	if err := s.store.UpdateUpgradePlan(ctx, plan); err != nil {
		s.logger.Error("Failed to update rollback status", "error", err)
	}

	// Restore from backup
	if err := s.restoreFromBackup(ctx, plan); err != nil {
		return fmt.Errorf("failed to restore from backup: %w", err)
	}

	s.logger.Info("Rollback completed")
	return nil
}

// restoreFromBackup restores the system from a backup
func (s *Service) restoreFromBackup(ctx context.Context, plan *UpgradePlan) error {
	s.logger.Info("Restoring from backup", "backupPath", plan.BackupPath)

	// This would restore all the backed up files and database
	// For now, just simulate the restore

	return nil
}

// GetUpgradeStatus returns the current status of an upgrade
func (s *Service) GetUpgradeStatus(ctx context.Context, planID string) (*UpgradePlan, error) {
	return s.store.GetUpgradePlan(ctx, planID)
}

// ListUpgradeHistory returns the upgrade history
func (s *Service) ListUpgradeHistory(ctx context.Context, limit int) ([]UpgradePlan, error) {
	return s.store.ListUpgradePlans(ctx, limit)
}

// CancelUpgrade cancels an ongoing upgrade
func (s *Service) CancelUpgrade(ctx context.Context, planID string) error {
	plan, err := s.store.GetUpgradePlan(ctx, planID)
	if err != nil {
		return fmt.Errorf("failed to get upgrade plan: %w", err)
	}

	if plan == nil {
		return fmt.Errorf("upgrade plan not found: %s", planID)
	}

	// Only allow cancellation of pending or in-progress upgrades
	if plan.Status != UpgradeStatusPending && plan.Status != UpgradeStatusUpgrading {
		return fmt.Errorf("cannot cancel upgrade with status: %s", plan.Status)
	}

	// Update plan status
	plan.Status = UpgradeStatusFailed
	plan.Error = "Upgrade cancelled by user"
	plan.UpdatedAt = time.Now().UTC()

	return s.store.UpdateUpgradePlan(ctx, plan)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, content, 0644)
}

// RunDatabaseMigrations runs database migrations as part of upgrade
func (s *Service) RunDatabaseMigrations(ctx context.Context) error {
	s.logger.Info("Running database migrations")

	// This would execute the database migration scripts
	// For now, just simulate the migration

	return nil
}

// VerifyUpgrade verifies that an upgrade was successful
func (s *Service) VerifyUpgrade(ctx context.Context, plan *UpgradePlan) error {
	s.logger.Info("Verifying upgrade")

	// Check that all components are running and healthy
	for _, component := range plan.Components {
		if err := s.verifyComponentHealth(ctx, component); err != nil {
			return fmt.Errorf("component %s is not healthy: %w", component, err)
		}
	}

	return nil
}

// verifyComponentHealth verifies that a component is healthy
func (s *Service) verifyComponentHealth(ctx context.Context, component string) error {
	// This would check the health of the component
	// For now, just return success
	return nil
}

// GetSystemStatus returns the overall system status
func (s *Service) GetSystemStatus(ctx context.Context) (map[string]string, error) {
	status := make(map[string]string)

	// Check each component
	components := []string{"api", "web", "beacon", "database"}
	for _, component := range components {
		version, err := s.GetCurrentVersion(component)
		if err != nil {
			status[component] = "unknown"
		} else {
			status[component] = version
		}
	}

	return status, nil
}
