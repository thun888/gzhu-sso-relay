package model

// Scope 的主键即 OAuth 线路上传输的 scope 标识，形如 read:profile。
type Scope struct {
	Name        string `gorm:"primaryKey;size:64"`
	DisplayName string `gorm:"size:100;not null"`
	Description string `gorm:"size:255"`
	IsDefault   bool   `gorm:"not null;default:false"`
	Timestamps
}

type ClientAllowedScope struct {
	ID        uint64 `gorm:"primaryKey"`
	ClientID  string `gorm:"size:64;not null;uniqueIndex:idx_client_allowed_scopes,priority:1"`
	ScopeName string `gorm:"size:64;not null;uniqueIndex:idx_client_allowed_scopes,priority:2"`
	Timestamps
}
