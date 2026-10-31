package model

type OAuthClient struct {
	ID           uint64   `gorm:"primaryKey"`
	ClientID     string   `gorm:"size:64;not null;uniqueIndex"`
	SecretHash   string   `gorm:"size:255;not null"`
	Name         string   `gorm:"size:128;not null"`
	RedirectURLs []string `gorm:"type:jsonb;serializer:json;not null"`
	// RequiredBindings 列出授权前必须完成的第三方账号绑定，如 ["gitea"]。
	RequiredBindings []string `gorm:"type:jsonb;serializer:json"`
	IsPublic         bool     `gorm:"not null;default:false"`
	// IsPrivate 为 true 时只有用户白名单或限定用户组里的账号能授权。
	IsPrivate bool `gorm:"not null;default:false"`
	// Enabled 为 false 时拒绝授权与令牌请求，记录保留以便随时恢复。
	Enabled     bool   `gorm:"not null;default:true"`
	HomepageURL string `gorm:"size:255"`
	Description string `gorm:"size:255"`
	// LogoType 非空表示 data/logos 下有同名文件，内容类型用于响应头。
	LogoType string `gorm:"size:64"`
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

// ClientUserGrant 是应用的用户白名单，只在 IsPrivate 为 true 时生效。
type ClientUserGrant struct {
	ID       uint64 `gorm:"primaryKey"`
	ClientID string `gorm:"size:64;not null;uniqueIndex:idx_client_user_grants,priority:1"`
	UserID   string `gorm:"size:32;not null;uniqueIndex:idx_client_user_grants,priority:2;index"`
	Timestamps
}

func (ClientUserGrant) TableName() string { return "client_user_grants" }
