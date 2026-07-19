package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

type AuditLog struct {
	UUIDModel
	UserID       string          `json:"user_id" gorm:"type:uuid;index:idx_audit_logs_user_action;not null"`
	Action       string          `json:"action" gorm:"type:text;not null"`
	ResourceType string          `json:"resource_type" gorm:"type:text;index:idx_audit_logs_resource;not null"`
	ResourceID   string          `json:"resource_id" gorm:"type:text;index:idx_audit_logs_resource"`
	Details      JSONMap         `json:"details" gorm:"type:jsonb"`
	IPAddress    string          `json:"ip_address" gorm:"type:text"`
	UserAgent    string          `json:"user_agent" gorm:"type:text"`
	CreatedAt    time.Time       `json:"created_at" gorm:"autoCreateTime;index:idx_audit_logs_created"`
}

type JSONMap map[string]interface{}

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONMap) Scan(src any) error {
	if src == nil {
		*j = nil
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return errors.New("models: unsupported scan source for JSONMap")
	}
	if len(raw) == 0 {
		*j = nil
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	*j = m
	return nil
}

func (a *AuditLog) Validate() error {
	if a.UserID == "" {
		return errors.New("user_id is required")
	}
	if a.Action == "" {
		return errors.New("action is required")
	}
	if a.ResourceType == "" {
		return errors.New("resource_type is required")
	}
	return nil
}

func (a *AuditLog) TableName() string {
	return "audit_logs"
}
