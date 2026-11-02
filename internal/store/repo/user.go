package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

var ErrNotFound = errors.New("repo: 记录不存在")

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) ByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: 查询用户 %s: %w", id, err)
	}
	return &u, nil
}

func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return fmt.Errorf("repo: 创建用户 %s: %w", u.ID, err)
	}
	return nil
}

// UpdatePasswordHash 只在上游验证通过后调用，一并清除过期标记。
func (r *UserRepo) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", id).
		Updates(map[string]any{"password_hash": hash, "upstream_stale": false}).Error
	if err != nil {
		return fmt.Errorf("repo: 更新用户 %s 密码哈希: %w", id, err)
	}
	return nil
}

func (r *UserRepo) SetUpstreamStale(ctx context.Context, id string, stale bool) error {
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", id).
		Update("upstream_stale", stale).Error
	if err != nil {
		return fmt.Errorf("repo: 更新用户 %s 上游验证标记: %w", id, err)
	}
	return nil
}

func (r *UserRepo) TouchLogin(ctx context.Context, id string, at time.Time) error {
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", id).
		Update("last_login_at", at).Error
	if err != nil {
		return fmt.Errorf("repo: 更新用户 %s 登录时间: %w", id, err)
	}
	return nil
}

func (r *UserRepo) EmailTaken(ctx context.Context, email, excludeID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("email = ? AND id <> ?", email, excludeID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("repo: 检查邮箱占用: %w", err)
	}
	return count > 0, nil
}

func (r *UserRepo) UpdateEmail(ctx context.Context, id, email string) error {
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", id).
		Update("email", email).Error
	if err != nil {
		return fmt.Errorf("repo: 更新用户 %s 邮箱: %w", id, err)
	}
	return nil
}

// Purge 删除账号及其全部关联记录，用于用户自助注销。
func (r *UserRepo) Purge(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, table := range []any{
			&model.OAuthAccount{},
			&model.UserGroup{},
			&model.Consent{},
			&model.ClientUserGrant{},
			&model.LoginNotice{},
		} {
			if err := tx.Where("user_id = ?", id).Delete(table).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", id).Delete(&model.User{}).Error
	})
	if err != nil {
		return fmt.Errorf("repo: 删除用户 %s: %w", id, err)
	}
	return nil
}

type UserFilter struct {
	Keyword string
	Type    *int16
	Status  *int16
	GroupID uint64
}

func (r *UserRepo) List(ctx context.Context, filter UserFilter, limit int) ([]model.User, error) {
	query := r.db.WithContext(ctx).Model(&model.User{}).Order("users.id").Limit(limit)
	if filter.Keyword != "" {
		like := "%" + filter.Keyword + "%"
		query = query.Where("users.id LIKE ? OR users.name LIKE ?", like, like)
	}
	if filter.Type != nil {
		query = query.Where("users.type = ?", *filter.Type)
	}
	if filter.Status != nil {
		query = query.Where("users.status = ?", *filter.Status)
	}
	if filter.GroupID > 0 {
		members := r.db.Model(&model.UserGroup{}).Select("user_id").Where("group_id = ?", filter.GroupID)
		query = query.Where("users.id IN (?)", members)
	}

	var users []model.User
	if err := query.Find(&users).Error; err != nil {
		return nil, fmt.Errorf("repo: 列出用户: %w", err)
	}
	return users, nil
}

func (r *UserRepo) UpdateFlags(ctx context.Context, id string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(fields).Error
	if err != nil {
		return fmt.Errorf("repo: 更新用户 %s: %w", id, err)
	}
	return nil
}

// MissingUsers 返回给定学号中尚不存在的部分，用于在写关联前校验。
func (r *UserRepo) MissingUsers(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var found []string
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id IN ?", ids).
		Pluck("id", &found).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 校验用户存在性: %w", err)
	}

	exists := make(map[string]struct{}, len(found))
	for _, id := range found {
		exists[id] = struct{}{}
	}

	var missing []string
	for _, id := range ids {
		if _, ok := exists[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// MemberViews 按给定学号取用户概要，顺序与入参一致，已删除的学号会被跳过。
func (r *UserRepo) MemberViews(ctx context.Context, ids []string) ([]MemberView, error) {
	if len(ids) == 0 {
		return []MemberView{}, nil
	}

	var users []model.User
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("repo: 查询用户概要: %w", err)
	}

	byID := make(map[string]model.User, len(users))
	for _, user := range users {
		byID[user.ID] = user
	}

	out := make([]MemberView, 0, len(ids))
	for _, id := range ids {
		user, ok := byID[id]
		if !ok {
			continue
		}
		out = append(out, MemberView{ID: user.ID, Name: user.Name, Email: user.Email, Status: user.Status})
	}
	return out, nil
}
