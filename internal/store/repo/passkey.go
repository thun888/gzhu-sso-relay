package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

type PasskeyRepo struct {
	db *gorm.DB
}

func NewPasskeyRepo(db *gorm.DB) *PasskeyRepo {
	return &PasskeyRepo{db: db}
}

func (r *PasskeyRepo) Create(ctx context.Context, p *model.Passkey) error {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return fmt.Errorf("repo: 创建通行密钥 %s: %w", p.UserID, err)
	}
	return nil
}

func (r *PasskeyRepo) ListByUser(ctx context.Context, userID string) ([]model.Passkey, error) {
	var list []model.Passkey
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at").
		Find(&list).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询用户 %s 的通行密钥: %w", userID, err)
	}
	return list, nil
}

func (r *PasskeyRepo) CountByUser(ctx context.Context, userID string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Passkey{}).Where("user_id = ?", userID).Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("repo: 统计用户 %s 的通行密钥: %w", userID, err)
	}
	return n, nil
}

func (r *PasskeyRepo) ByCredentialID(ctx context.Context, credentialID string) (*model.Passkey, error) {
	var p model.Passkey
	err := r.db.WithContext(ctx).Where("credential_id = ?", credentialID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: 查询通行密钥 %s: %w", credentialID, err)
	}
	return &p, nil
}

// Touch 在每次断言通过后回写计数器与时间戳，不改动设备名与创建时间。
func (r *PasskeyRepo) Touch(ctx context.Context, id, credential string, signCount uint32, usedAt time.Time) error {
	err := r.db.WithContext(ctx).Model(&model.Passkey{}).Where("id = ?", id).Updates(map[string]any{
		"credential":   credential,
		"sign_count":   signCount,
		"last_used_at": usedAt,
	}).Error
	if err != nil {
		return fmt.Errorf("repo: 更新通行密钥 %s: %w", id, err)
	}
	return nil
}

// Delete 按用户与主键删除，避免用别人的设备 id 解绑。
func (r *PasskeyRepo) Delete(ctx context.Context, userID, id string) (bool, error) {
	result := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&model.Passkey{})
	if result.Error != nil {
		return false, fmt.Errorf("repo: 删除通行密钥 %s: %w", id, result.Error)
	}
	return result.RowsAffected > 0, nil
}
