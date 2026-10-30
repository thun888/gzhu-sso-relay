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

func (r *UserRepo) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", id).
		Update("password_hash", hash).Error
	if err != nil {
		return fmt.Errorf("repo: 更新用户 %s 密码哈希: %w", id, err)
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
