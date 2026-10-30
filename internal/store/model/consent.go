package model

// Consent 记录用户对某个应用授予过的 scope 集合，授权端点据此判断是否需要重新征求同意。
type Consent struct {
	ID       uint64   `gorm:"primaryKey"`
	UserID   string   `gorm:"size:32;not null;uniqueIndex:idx_consents_user_client,priority:1"`
	ClientID string   `gorm:"size:64;not null;uniqueIndex:idx_consents_user_client,priority:2"`
	Scopes   []string `gorm:"type:jsonb;serializer:json;not null"`
	Timestamps
}
