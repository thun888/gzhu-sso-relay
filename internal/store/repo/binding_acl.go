package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

type BindingACLRepo struct {
	db *gorm.DB
}

func NewBindingACLRepo(db *gorm.DB) *BindingACLRepo {
	return &BindingACLRepo{db: db}
}

// All 返回全部绑定源的访问控制配置。没有记录的来源不在结果里。
func (r *BindingACLRepo) All(ctx context.Context) (map[string]model.BindingACL, error) {
	var rows []model.BindingACL
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("repo: 列出绑定源访问控制: %w", err)
	}

	out := make(map[string]model.BindingACL, len(rows))
	for _, row := range rows {
		out[row.Provider] = row
	}
	return out, nil
}

func (r *BindingACLRepo) Get(ctx context.Context, provider string) (model.BindingACL, bool, error) {
	var row model.BindingACL
	err := r.db.WithContext(ctx).Where("provider = ?", provider).First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return model.BindingACL{}, false, nil
	}
	if err != nil {
		return model.BindingACL{}, false, fmt.Errorf("repo: 查询绑定源 %s 的访问控制: %w", provider, err)
	}
	return row, true, nil
}

// Save 按 provider 覆盖写入，未提供的列表按空处理。
func (r *BindingACLRepo) Save(ctx context.Context, acl model.BindingACL) error {
	if acl.GroupIDs == nil {
		acl.GroupIDs = []uint64{}
	}
	if acl.UserIDs == nil {
		acl.UserIDs = []string{}
	}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider"}},
		DoUpdates: clause.AssignmentColumns([]string{"is_private", "group_ids", "user_ids", "updated_at"}),
	}).Create(&acl).Error
	if err != nil {
		return fmt.Errorf("repo: 保存绑定源 %s 的访问控制: %w", acl.Provider, err)
	}
	return nil
}

// Allows 判断用户能否使用该绑定源。没有配置过的来源对所有登录用户开放。
func (r *BindingACLRepo) Allows(ctx context.Context, provider, userID string) (bool, error) {
	acl, ok, err := r.Get(ctx, provider)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return r.permits(ctx, acl, userID)
}

// AllowedSet 对一批绑定源逐项判定，返回可用的来源集合。
func (r *BindingACLRepo) AllowedSet(ctx context.Context, providers []string, userID string) (map[string]bool, error) {
	all, err := r.All(ctx)
	if err != nil {
		return nil, err
	}

	out := make(map[string]bool, len(providers))
	for _, provider := range providers {
		acl, ok := all[provider]
		if !ok {
			out[provider] = true
			continue
		}
		permitted, err := r.permits(ctx, acl, userID)
		if err != nil {
			return nil, err
		}
		out[provider] = permitted
	}
	return out, nil
}

func (r *BindingACLRepo) permits(ctx context.Context, acl model.BindingACL, userID string) (bool, error) {
	if !acl.IsPrivate {
		return true, nil
	}
	for _, id := range acl.UserIDs {
		if id == userID {
			return true, nil
		}
	}
	if len(acl.GroupIDs) == 0 {
		return false, nil
	}
	return userInAnyGroup(r.db, ctx, userID, acl.GroupIDs)
}
