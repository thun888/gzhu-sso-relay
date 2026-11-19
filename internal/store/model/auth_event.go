package model

import "time"

// AuthEvent 是只追加的登录与授权事件，不参与鉴权判断。
type AuthEvent struct {
	ID         uint64    `gorm:"primaryKey"`
	OccurredAt time.Time `gorm:"not null;index"`
	Kind       string    `gorm:"size:32;not null;index"`
	UserID     string    `gorm:"size:32;index"`
	ActorID    string    `gorm:"size:32"`
	ClientID   string    `gorm:"size:64;index"`
	ClientName string    `gorm:"size:128"`
	SessionID  string    `gorm:"size:36"`
	Method     string    `gorm:"size:16"`
	Result     string    `gorm:"size:16;not null"`
	Reason     string    `gorm:"size:64"`
	IP         string    `gorm:"size:64"`
	UserAgent  string    `gorm:"size:256"`
	Scopes     []string  `gorm:"type:jsonb;serializer:json"`
	Detail     string    `gorm:"type:text"`
}

func (AuthEvent) TableName() string { return "auth_events" }
