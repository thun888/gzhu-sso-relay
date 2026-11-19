package model

import "time"

// Consent 记录用户对某个应用授予过的 scope 集合，授权端点据此判断是否需要重新征求同意。
// 撤销只标记 revoked_at 并递增 version，已签发的访问令牌靠 version 判定失效。
type Consent struct {
	ID         uint64   `gorm:"primaryKey"`
	UserID     string   `gorm:"size:32;not null;uniqueIndex:idx_consents_user_client,priority:1"`
	ClientID   string   `gorm:"size:64;not null;uniqueIndex:idx_consents_user_client,priority:2"`
	Scopes     []string `gorm:"type:jsonb;serializer:json;not null"`
	Version    int64    `gorm:"not null;default:1"`
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	Timestamps
}
