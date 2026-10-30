package model

type OAuthClient struct {
	ID           uint64   `gorm:"primaryKey"`
	ClientID     string   `gorm:"size:64;not null;uniqueIndex"`
	SecretHash   string   `gorm:"size:255;not null"`
	Name         string   `gorm:"size:128;not null"`
	RedirectURLs []string `gorm:"type:jsonb;serializer:json;not null"`
	IsPublic     bool     `gorm:"not null;default:false"`
	Timestamps
}

func (OAuthClient) TableName() string { return "oauth_clients" }

// GroupOAuthClient 为空表示该应用不限制用户组，任何已登录用户都可授权。
type GroupOAuthClient struct {
	ID       uint64 `gorm:"primaryKey"`
	GroupID  uint64 `gorm:"not null;uniqueIndex:idx_group_oauth_clients,priority:1;index"`
	ClientID string `gorm:"size:64;not null;uniqueIndex:idx_group_oauth_clients,priority:2"`
	Timestamps
}

func (GroupOAuthClient) TableName() string { return "group_oauth_clients" }
