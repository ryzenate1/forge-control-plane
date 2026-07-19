package store

import (
	"time"
)

type Backup struct {
	UUID        string     `json:"uuid"`
	ServerID    string     `json:"serverId"`
	Name        string     `json:"name"`
	Checksum    string     `json:"checksum"`
	Size        int64      `json:"size"`
	Status      string     `json:"status"`
	UploadID    *string    `json:"uploadId,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	IsLocked    bool       `json:"isLocked"`
	// Status tracking fields
	StatusMessage  *string    `json:"statusMessage,omitempty"`
	StatusCallback *string    `json:"statusCallback,omitempty"`
	RetryCount     int        `json:"retryCount"`
	LastRetryAt    *time.Time `json:"lastRetryAt,omitempty"`
}

type UpsertBackupRequest struct {
	UUID           string
	Name           string
	Checksum       string
	Size           int64
	Status         string
	UploadID       *string
	CompletedAt    *time.Time
	StatusMessage  *string
	StatusCallback *string
	RetryCount     int
}
