package model

type Group struct {
	ID       uint64  `gorm:"primaryKey"`
	Name     string  `gorm:"size:64;not null"`
	Code     string  `gorm:"size:64;not null;uniqueIndex"`
	ParentID *uint64 `gorm:"index"`
	Timestamps
}

type UserGroup struct {
	ID      uint64 `gorm:"primaryKey"`
	UserID  string `gorm:"size:32;not null;uniqueIndex:idx_user_groups,priority:1"`
	GroupID uint64 `gorm:"not null;uniqueIndex:idx_user_groups,priority:2;index"`
	Timestamps
}
