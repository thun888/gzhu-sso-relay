package model

// BindingACL 控制谁能在用户中心看到并使用某个绑定源。Provider 对应 config.yaml 里
// bindings.providers 的 name；没有对应行的来源对所有登录用户开放。
type BindingACL struct {
	ID        uint64   `gorm:"primaryKey"`
	Provider  string   `gorm:"size:64;not null;uniqueIndex"`
	IsPrivate bool     `gorm:"not null;default:false"`
	GroupIDs  []uint64 `gorm:"type:jsonb;serializer:json"`
	UserIDs   []string `gorm:"type:jsonb;serializer:json"`
	Timestamps
}

func (BindingACL) TableName() string { return "binding_acls" }
