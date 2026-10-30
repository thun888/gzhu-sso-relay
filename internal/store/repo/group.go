package repo

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

type GroupRepo struct {
	db *gorm.DB
}

func NewGroupRepo(db *gorm.DB) *GroupRepo {
	return &GroupRepo{db: db}
}

func (r *GroupRepo) List(ctx context.Context) ([]model.Group, error) {
	var groups []model.Group
	if err := r.db.WithContext(ctx).Order("id").Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("repo: 列出用户组: %w", err)
	}
	return groups, nil
}

func (r *GroupRepo) ByID(ctx context.Context, id uint64) (*model.Group, error) {
	var g model.Group
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: 查询用户组 %d: %w", id, err)
	}
	return &g, nil
}

func (r *GroupRepo) Create(ctx context.Context, g *model.Group) error {
	if err := r.db.WithContext(ctx).Create(g).Error; err != nil {
		return fmt.Errorf("repo: 创建用户组 %s: %w", g.Code, err)
	}
	return nil
}

func (r *GroupRepo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.Group{}).Where("id = ?", id).Updates(fields).Error
	if err != nil {
		return fmt.Errorf("repo: 更新用户组 %d: %w", id, err)
	}
	return nil
}

func (r *GroupRepo) Delete(ctx context.Context, id uint64) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", id).Delete(&model.UserGroup{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&model.GroupOAuthClient{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.Group{}).Error
	})
	if err != nil {
		return fmt.Errorf("repo: 删除用户组 %d: %w", id, err)
	}
	return nil
}

func (r *GroupRepo) Members(ctx context.Context, groupID uint64) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&model.UserGroup{}).
		Where("group_id = ?", groupID).
		Order("user_id").
		Pluck("user_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 列出用户组 %d 的成员: %w", groupID, err)
	}
	return ids, nil
}

func (r *GroupRepo) SetMembers(ctx context.Context, groupID uint64, userIDs []string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&model.UserGroup{}).Error; err != nil {
			return err
		}
		if len(userIDs) == 0 {
			return nil
		}
		links := make([]model.UserGroup, 0, len(userIDs))
		for _, id := range userIDs {
			links = append(links, model.UserGroup{UserID: id, GroupID: groupID})
		}
		return tx.Create(&links).Error
	})
	if err != nil {
		return fmt.Errorf("repo: 设置用户组 %d 的成员: %w", groupID, err)
	}
	return nil
}

func (r *GroupRepo) OfUser(ctx context.Context, userID string) ([]model.Group, error) {
	var groups []model.Group
	err := r.db.WithContext(ctx).
		Joins("JOIN user_groups ON user_groups.group_id = groups.id").
		Where("user_groups.user_id = ?", userID).
		Order("groups.id").
		Find(&groups).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询用户 %s 的用户组: %w", userID, err)
	}
	return groups, nil
}
