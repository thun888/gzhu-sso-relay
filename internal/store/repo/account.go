package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thun888/gzhu-sso-relay/internal/cryptox"
	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

// UpstreamProvider 是广州大学统一认证在 oauth_accounts 中的 provider 标识。
const UpstreamProvider = "gzhu"

type AccountRepo struct {
	db     *gorm.DB
	cipher *cryptox.Cipher
}

func NewAccountRepo(db *gorm.DB, cipher *cryptox.Cipher) *AccountRepo {
	return &AccountRepo{db: db, cipher: cipher}
}

// sealProfile 加密快照；openProfile 解密 v1 密文，无前缀的历史明文原样返回。
func (r *AccountRepo) sealProfile(raw []byte) string {
	return r.cipher.Seal(raw)
}

func (r *AccountRepo) openProfile(stored string) ([]byte, error) {
	if !strings.HasPrefix(stored, "v1:") {
		return []byte(stored), nil
	}
	plain, err := r.cipher.Open(stored)
	if err != nil {
		return nil, fmt.Errorf("repo: 解密 profile 快照: %w", err)
	}
	return plain, nil
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

// ByProviderUserID 按上游身份标识反查绑定，扫码登录拿不到本地账号时靠它定位用户。
func (r *AccountRepo) ByProviderUserID(ctx context.Context, provider, providerUserID string) (*model.OAuthAccount, error) {
	var a model.OAuthAccount
	err := r.db.WithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", provider, providerUserID).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: 查询上游账号 %s/%s: %w", provider, providerUserID, err)
	}
	return &a, nil
}

// SaveProfile 写入或刷新 profile_data 快照，降级时由它代替上游返回。
func (r *AccountRepo) SaveProfile(ctx context.Context, userID, provider, providerUserID string, profile any) error {
	raw, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("repo: 序列化 profile: %w", err)
	}
	sealed := r.sealProfile(raw)

	account := model.OAuthAccount{
		UserID:         userID,
		Provider:       provider,
		ProviderUserID: providerUserID,
		ProfileData:    sealed,
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

func (r *AccountRepo) ProfileData(ctx context.Context, userID, provider string) ([]byte, error) {
	var a model.OAuthAccount
	err := r.db.WithContext(ctx).
		Select("profile_data").
		Where("user_id = ? AND provider = ?", userID, provider).
		Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo: 读取 %s/%s 的 profile_data: %w", userID, provider, err)
	}
	if a.ProfileData == "" {
		return nil, nil
	}
	return r.openProfile(a.ProfileData)
}

// ClassOf 从快照里取班级，快照缺失或未记录时返回空串。
func (r *AccountRepo) ClassOf(ctx context.Context, userID, provider string) string {
	raw, err := r.ProfileData(ctx, userID, provider)
	if err != nil || len(raw) == 0 {
		return ""
	}

	var snapshot struct {
		Class string `json:"belong_class"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return ""
	}
	return snapshot.Class
}

type BindingProfile struct {
	DisplayName string          `json:"display_name"`
	Raw         json.RawMessage `json:"raw"`
}

type Binding struct {
	Provider       string    `json:"provider"`
	ProviderUserID string    `json:"provider_user_id"`
	DisplayName    string    `json:"display_name"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Bind 写入第三方账号绑定，同一用户同一来源只保留一条。
func (r *AccountRepo) Bind(ctx context.Context, userID, provider, providerUserID, displayName string, raw []byte) error {
	profile, err := json.Marshal(BindingProfile{DisplayName: displayName, Raw: raw})
	if err != nil {
		return fmt.Errorf("repo: 序列化绑定快照: %w", err)
	}
	sealed := r.sealProfile(profile)

	account := model.OAuthAccount{
		UserID:         userID,
		Provider:       provider,
		ProviderUserID: providerUserID,
		ProfileData:    sealed,
	}

	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "provider"}},
		DoUpdates: clause.AssignmentColumns([]string{"provider_user_id", "profile_data", "updated_at"}),
	}).Create(&account).Error
	if err != nil {
		return fmt.Errorf("repo: 写入绑定 %s/%s: %w", userID, provider, err)
	}
	return nil
}

func (r *AccountRepo) Unbind(ctx context.Context, userID, provider string) error {
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND provider = ?", userID, provider).
		Delete(&model.OAuthAccount{}).Error
	if err != nil {
		return fmt.Errorf("repo: 解除绑定 %s/%s: %w", userID, provider, err)
	}
	return nil
}

func (r *AccountRepo) BindingsOf(ctx context.Context, userID string) ([]Binding, error) {
	var accounts []model.OAuthAccount
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("provider").
		Find(&accounts).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询 %s 的绑定: %w", userID, err)
	}

	out := make([]Binding, 0, len(accounts))
	for _, account := range accounts {
		display := account.ProviderUserID
		var profile BindingProfile
		if account.ProfileData != "" {
			if raw, err := r.openProfile(account.ProfileData); err == nil {
				if json.Unmarshal(raw, &profile) == nil && profile.DisplayName != "" {
					display = profile.DisplayName
				}
			}
		}
		out = append(out, Binding{
			Provider:       account.Provider,
			ProviderUserID: account.ProviderUserID,
			DisplayName:    display,
			UpdatedAt:      account.UpdatedAt,
		})
	}
	return out, nil
}

func (r *AccountRepo) HasBinding(ctx context.Context, userID, provider string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.OAuthAccount{}).
		Where("user_id = ? AND provider = ?", userID, provider).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("repo: 检查 %s/%s 的绑定: %w", userID, provider, err)
	}
	return count > 0, nil
}
