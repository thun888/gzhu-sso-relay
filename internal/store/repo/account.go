package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

// UpstreamProvider 是广州大学统一认证在 oauth_accounts 中的 provider 标识。
const UpstreamProvider = "gzhu"

type AccountRepo struct {
	db *gorm.DB
}

func NewAccountRepo(db *gorm.DB) *AccountRepo {
	return &AccountRepo{db: db}
}

func (r *AccountRepo) ByUserAndProvider(ctx context.Context, userID, provider string) (*model.OAuthAccount, error) {
	var a model.OAuthAccount
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND provider = ?", userID, provider).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: 查询绑定 %s/%s: %w", userID, provider, err)
	}
	return &a, nil
}

// SaveProfile 写入或刷新 profile_data 快照，降级时由它代替上游返回。
func (r *AccountRepo) SaveProfile(ctx context.Context, userID, provider, providerUserID string, profile any) error {
	raw, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("repo: 序列化 profile: %w", err)
	}

	account := model.OAuthAccount{
		UserID:         userID,
		Provider:       provider,
		ProviderUserID: providerUserID,
		ProfileData:    raw,
	}

	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "provider"}},
		DoUpdates: clause.AssignmentColumns([]string{"provider_user_id", "profile_data", "updated_at"}),
	}).Create(&account).Error
	if err != nil {
		return fmt.Errorf("repo: 保存绑定 %s/%s: %w", userID, provider, err)
	}
	return nil
}
