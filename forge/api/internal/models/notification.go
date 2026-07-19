package models

import (
	"errors"
	"time"
)

type Notification struct {
	UUIDModel
	UserID    string     `json:"user_id" gorm:"type:uuid;index:idx_notifications_user_read;not null"`
	Type      string     `json:"type" gorm:"type:text;not null"`
	Title     string     `json:"title" gorm:"type:text;not null"`
	Body      string     `json:"body" gorm:"type:text"`
	Read      bool       `json:"read" gorm:"default:false;index:idx_notifications_user_read"`
	Metadata  JSONMap    `json:"metadata" gorm:"type:jsonb"`
	CreatedAt time.Time  `json:"created_at" gorm:"autoCreateTime;index"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

func (n *Notification) MarkRead() {
	n.Read = true
	now := time.Now()
	n.ReadAt = &now
}

func (n *Notification) Validate() error {
	if n.UserID == "" {
		return errors.New("user_id is required")
	}
	if n.Type == "" {
		return errors.New("type is required")
	}
	if n.Title == "" {
		return errors.New("title is required")
	}
	return nil
}

func (n *Notification) TableName() string {
	return "notifications"
}

type NotificationPreference struct {
	UUIDModel
	UserID    string `json:"user_id" gorm:"type:uuid;index:idx_notif_prefs_user;not null"`
	Channel   string `json:"channel" gorm:"type:text;not null"`
	EventType string `json:"event_type" gorm:"type:text;not null"`
	Enabled   bool   `json:"enabled" gorm:"default:true"`
}

func (np *NotificationPreference) Validate() error {
	if np.UserID == "" {
		return errors.New("user_id is required")
	}
	switch np.Channel {
	case "email", "webhook", "push":
	default:
		return errors.New("channel must be email, webhook, or push")
	}
	if np.EventType == "" {
		return errors.New("event_type is required")
	}
	return nil
}

func (np *NotificationPreference) TableName() string {
	return "notification_preferences"
}
