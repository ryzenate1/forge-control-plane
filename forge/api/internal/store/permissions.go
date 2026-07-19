package store

// Granular permissions stored as JSON arrays in the subusers.permissions column.

const (
	PermWebsocketConnect = "websocket.connect"

	PermControlConsole = "control.console"
	PermControlStart   = "control.start"
	PermControlStop    = "control.stop"
	PermControlRestart = "control.restart"

	PermDatabaseRead         = "database.read"
	PermDatabaseCreate       = "database.create"
	PermDatabaseUpdate       = "database.update"
	PermDatabaseDelete       = "database.delete"
	PermDatabaseViewPassword = "database.view_password"

	PermScheduleRead   = "schedule.read"
	PermScheduleCreate = "schedule.create"
	PermScheduleUpdate = "schedule.update"
	PermScheduleDelete = "schedule.delete"

	PermUserRead   = "user.read"
	PermUserCreate = "user.create"
	PermUserUpdate = "user.update"
	PermUserDelete = "user.delete"

	PermBackupRead     = "backup.read"
	PermBackupCreate   = "backup.create"
	PermBackupDelete   = "backup.delete"
	PermBackupDownload = "backup.download"
	PermBackupRestore  = "backup.restore"

	PermAllocationRead   = "allocation.read"
	PermAllocationCreate = "allocation.create"
	PermAllocationUpdate = "allocation.update"
	PermAllocationDelete = "allocation.delete"

	PermFileRead        = "file.read"
	PermFileReadContent = "file.read-content"
	PermFileCreate      = "file.create"
	PermFileUpdate      = "file.update"
	PermFileDelete      = "file.delete"
	PermFileArchive     = "file.archive"
	PermFileSFTP        = "file.sftp"

	PermStartupRead        = "startup.read"
	PermStartupUpdate      = "startup.update"
	PermStartupDockerImage = "startup.docker-image"

	PermSettingsRename      = "settings.rename"
	PermSettingsReinstall   = "settings.reinstall"
	PermSettingsDescription = "settings.description"
	PermSettingsChangeIcon  = "settings.change-icon"

	PermMountRead   = "mount.read"
	PermMountUpdate = "mount.update"

	PermActivityRead = "activity.read"

	PermServerView     = "server.view"
	PermServerSettings = "server.settings"
)

// AllPermissions returns every defined permission key.
func AllPermissions() []string {
	return []string{
		PermWebsocketConnect,
		PermControlConsole, PermControlStart, PermControlStop, PermControlRestart,
		PermDatabaseRead, PermDatabaseCreate, PermDatabaseUpdate, PermDatabaseDelete, PermDatabaseViewPassword,
		PermScheduleRead, PermScheduleCreate, PermScheduleUpdate, PermScheduleDelete,
		PermUserRead, PermUserCreate, PermUserUpdate, PermUserDelete,
		PermBackupRead, PermBackupCreate, PermBackupDelete, PermBackupDownload, PermBackupRestore,
		PermAllocationRead, PermAllocationCreate, PermAllocationUpdate, PermAllocationDelete,
		PermFileRead, PermFileReadContent, PermFileCreate, PermFileUpdate, PermFileDelete, PermFileArchive, PermFileSFTP,
		PermStartupRead, PermStartupUpdate, PermStartupDockerImage,
		PermSettingsRename, PermSettingsReinstall, PermSettingsDescription, PermSettingsChangeIcon,
		PermMountRead, PermMountUpdate,
		PermActivityRead,
	}
}

// PermissionDescriptions returns human-readable descriptions for the permission groups.
func PermissionDescriptions() map[string]map[string]string {
	return map[string]map[string]string{
		"websocket": {
			"connect": "Allows the user to connect to the server websocket for console and stats.",
		},
		"control": {
			"console": "Allows a user to send commands to the server console.",
			"start":   "Allows a user to start the server.",
			"stop":    "Allows a user to stop the server.",
			"restart": "Allows a user to restart the server.",
		},
		"user": {
			"create": "Allows a user to create new subusers.",
			"read":   "Allows viewing subusers and their permissions.",
			"update": "Allows modifying other subusers.",
			"delete": "Allows deleting a subuser from the server.",
		},
		"file": {
			"create":       "Allows creating files and folders or uploading.",
			"read":         "Allows viewing directory contents.",
			"read-content": "Allows viewing file contents and downloading.",
			"update":       "Allows updating existing files.",
			"delete":       "Allows deleting files or directories.",
			"archive":      "Allows archiving and decompressing files.",
			"sftp":         "Allows SFTP access to server files.",
		},
		"backup": {
			"create":   "Allows creating backups.",
			"read":     "Allows viewing backups.",
			"delete":   "Allows removing backups.",
			"download": "Allows downloading backups.",
			"restore":  "Allows restoring backups.",
		},
		"allocation": {
			"read":   "Allows viewing server allocations.",
			"create": "Allows assigning additional allocations.",
			"update": "Allows changing the primary allocation.",
			"delete": "Allows removing an allocation.",
		},
		"startup": {
			"read":         "Allows viewing startup variables.",
			"update":       "Allows modifying startup variables.",
			"docker-image": "Allows changing the Docker image.",
		},
		"database": {
			"create":        "Allows creating databases.",
			"read":          "Allows viewing databases.",
			"update":        "Allows rotating database passwords.",
			"delete":        "Allows removing databases.",
			"view_password": "Allows viewing database passwords.",
		},
		"schedule": {
			"create": "Allows creating schedules.",
			"read":   "Allows viewing schedules.",
			"update": "Allows updating schedules and tasks.",
			"delete": "Allows deleting schedules.",
		},
		"settings": {
			"rename":      "Allows renaming the server.",
			"reinstall":   "Allows triggering a server reinstall.",
			"description": "Allows changing the server description.",
			"change-icon": "Allows changing the server icon.",
		},
		"mount": {
			"read":   "Allows viewing server mounts.",
			"update": "Allows attaching and detaching mounts.",
		},
		"activity": {
			"read": "Allows viewing the server activity log.",
		},
	}
}

// HasPermission checks if a list of permissions contains a specific permission.
func HasPermission(permissions []string, required string) bool {
	for _, p := range permissions {
		if p == "*" || p == required {
			return true
		}
	}
	return false
}
