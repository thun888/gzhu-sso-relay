package model

// LoginNotice 记录已发送过首次登录通知的账号与应用组合，撤销授权后重新授权也不会重复发送。
type LoginNotice struct {
	ID       uint64 `gorm:"primaryKey"`
	UserID   string `gorm:"size:32;not null;uniqueIndex:idx_login_notices_user_client,priority:1"`
	ClientID string `gorm:"size:64;not null;uniqueIndex:idx_login_notices_user_client,priority:2"`
	Timestamps
}

func (LoginNotice) TableName() string { return "login_notices" }
