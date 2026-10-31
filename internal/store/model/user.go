package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	UserTypeNormal = 0
	UserTypeAdmin  = 1
)

const (
	UserStatusActive   = 0
	UserStatusDisabled = 1
)

type User struct {
	ID           string `gorm:"primaryKey;size:32"`
	Name         string `gorm:"size:64;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	// UpstreamStale 为 true 表示上次带本地哈希对应密码回访上游被拒，本地命中需同步复核后放行。
	UpstreamStale bool `gorm:"not null;default:false"`
	Email         string `gorm:"size:128;not null;index"`
	Type         int16  `gorm:"not null;default:0"`
	Status       int16  `gorm:"not null;default:0"`
	LastLoginAt  *time.Time
	Timestamps
}

// OAuthAccount 同时承担上游身份绑定与 profile_data 快照。
// AccessToken 与 RefreshToken 存 cryptox 密文，Provider 为 gitea 时 ProviderUserID 是 gitea 用户名。
type OAuthAccount struct {
	ID             string `gorm:"primaryKey;size:36"`
	UserID         string `gorm:"size:32;not null;uniqueIndex:idx_oauth_accounts_user_provider,priority:1"`
	Provider       string `gorm:"size:32;not null;uniqueIndex:idx_oauth_accounts_user_provider,priority:2;uniqueIndex:idx_oauth_accounts_provider_uid,priority:1"`
	ProviderUserID string `gorm:"size:128;not null;uniqueIndex:idx_oauth_accounts_provider_uid,priority:2"`
	AccessToken    string `gorm:"type:text"`
	RefreshToken   string `gorm:"type:text"`
	ProfileData    []byte `gorm:"type:jsonb"`
	Timestamps
}

func (OAuthAccount) TableName() string { return "oauth_accounts" }

func (a *OAuthAccount) BeforeCreate(*gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	return nil
}
