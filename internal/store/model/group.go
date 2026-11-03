package model

type Group struct {
	ID   uint64 `gorm:"primaryKey"`
	Name string `gorm:"size:64;not null"`
	Code string `gorm:"size:64;not null;uniqueIndex"`
	// IsUpstream 为 true 时该组由上游身份信息同步生成，成员不手工维护。
	IsUpstream bool    `gorm:"not null;default:false"`
	ParentID   *uint64 `gorm:"index"`
	Timestamps
}

type UserGroup struct {
	ID      uint64 `gorm:"primaryKey"`
	UserID  string `gorm:"size:32;not null;uniqueIndex:idx_user_groups,priority:1"`
	GroupID uint64 `gorm:"not null;uniqueIndex:idx_user_groups,priority:2;index"`
	Timestamps
}

type GroupLeader struct {
	ID      uint64 `gorm:"primaryKey"`
	GroupID uint64 `gorm:"not null;uniqueIndex:idx_group_leaders,priority:1;index"`
	UserID  string `gorm:"size:32;not null;uniqueIndex:idx_group_leaders,priority:2;index"`
	Timestamps
}
