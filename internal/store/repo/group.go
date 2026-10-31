package repo

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

type MemberView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Status int16  `json:"status"`
}

func (r *GroupRepo) MemberViews(ctx context.Context, groupID uint64) ([]MemberView, error) {
	members := make([]MemberView, 0)
	err := r.db.WithContext(ctx).
		Table("user_groups").
		Select("users.id, users.name, users.email, users.status").
		Joins("JOIN users ON users.id = user_groups.user_id").
		Where("user_groups.group_id = ?", groupID).
		Order("users.id").
		Scan(&members).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 列出用户组 %d 的成员: %w", groupID, err)
	}
	return members, nil
}

func (r *GroupRepo) AddMember(ctx context.Context, groupID uint64, userID string) error {
	link := model.UserGroup{UserID: userID, GroupID: groupID}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error
	if err != nil {
		return fmt.Errorf("repo: 把 %s 加入用户组 %d: %w", userID, groupID, err)
	}
	return nil
}

func (r *GroupRepo) RemoveMember(ctx context.Context, groupID uint64, userID string) error {
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Delete(&model.UserGroup{}).Error
	if err != nil {
		return fmt.Errorf("repo: 把 %s 移出用户组 %d: %w", userID, groupID, err)
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

type GroupSpec struct {
	Code string
	Name string
}

// SyncUpstream 用上游身份信息重建用户的同步组，手工维护的组不受影响。
func (r *GroupRepo) SyncUpstream(ctx context.Context, userID string, specs []GroupSpec) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := make([]uint64, 0, len(specs))
		for _, spec := range specs {
			group := model.Group{Name: spec.Name, Code: spec.Code, IsUpstream: true}
			err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "code"}},
				DoUpdates: clause.AssignmentColumns([]string{"name", "is_upstream", "updated_at"}),
			}).Create(&group).Error
			if err != nil {
				return err
			}
			ids = append(ids, group.ID)
		}

		upstream := tx.Model(&model.Group{}).Select("id").Where("is_upstream")
		if err := tx.Where("user_id = ? AND group_id IN (?)", userID, upstream).Delete(&model.UserGroup{}).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}

		links := make([]model.UserGroup, 0, len(ids))
		for _, id := range ids {
			links = append(links, model.UserGroup{UserID: userID, GroupID: id})
		}
		return tx.Create(&links).Error
	})
	if err != nil {
		return fmt.Errorf("repo: 同步用户 %s 的上游用户组: %w", userID, err)
	}
	return nil
}
