package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Passkey 是一台设备上的通行密钥，Credential 存 webauthn.Credential 的 JSON，
// SignCount 与 BackupEligible 是它的冗余字段，供列表与克隆检测使用。
type Passkey struct {
	ID             string `gorm:"primaryKey;size:36"`
	UserID         string `gorm:"size:32;not null;index"`
	CredentialID   string `gorm:"size:255;not null;uniqueIndex"`
	Name           string `gorm:"size:64;not null"`
	Credential     string `gorm:"type:text;not null"`
	SignCount      uint32 `gorm:"not null;default:0"`
	BackupEligible bool   `gorm:"not null;default:false"`
	LastUsedAt     *time.Time
	Timestamps
}

func (Passkey) TableName() string { return "passkeys" }

func (p *Passkey) BeforeCreate(*gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	return nil
}
