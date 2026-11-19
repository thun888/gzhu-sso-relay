package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

type ConsentRepo struct {
	db *gorm.DB
}

func NewConsentRepo(db *gorm.DB) *ConsentRepo {
	return &ConsentRepo{db: db}
}

type Authorization struct {
	ClientID    string
	Name        string
	HomepageURL string
	Scopes      []string
	GrantedAt   time.Time
	LastUsedAt  *time.Time
}

func (r *ConsentRepo) Scopes(ctx context.Context, userID, clientID string) ([]string, error) {
	record, err := r.find(ctx, userID, clientID)
	if err != nil || record == nil || record.RevokedAt != nil {
		return nil, err
	}
	return record.Scopes, nil
}

// Grant 写入或合并用户对应用的授权 scope。已撤销的记录重新授权时从空集合开始，不继承旧 scope。
func (r *ConsentRepo) Grant(ctx context.Context, userID, clientID string, scopes []string) error {
	existing, err := r.find(ctx, userID, clientID)
	if err != nil {
		return err
	}

	var merged []string
	if existing != nil && existing.RevokedAt == nil {
		merged = mergeScopes(existing.Scopes, scopes)
	} else {
		merged = mergeScopes(nil, scopes)
	}

	record := model.Consent{UserID: userID, ClientID: clientID, Scopes: merged, Version: 1}
	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"scopes", "revoked_at", "updated_at"}),
	}).Create(&record).Error
	if err != nil {
		return fmt.Errorf("repo: 写入授权记录 %s/%s: %w", userID, clientID, err)
	}
	return nil
}

func (r *ConsentRepo) Revoke(ctx context.Context, userID, clientID string) error {
	err := r.db.WithContext(ctx).Model(&model.Consent{}).
		Where("user_id = ? AND client_id = ? AND revoked_at IS NULL", userID, clientID).
		Updates(map[string]any{
			"revoked_at": time.Now(),
			"version":    gorm.Expr("GREATEST(version, 1) + 1"),
			"scopes":     gorm.Expr("'[]'::jsonb"),
		}).Error
	if err != nil {
		return fmt.Errorf("repo: 撤销授权记录 %s/%s: %w", userID, clientID, err)
	}
	return nil
}

// ActiveVersion 返回当前有效授权的版本号，撤销或不存在时 ok 为 false。
func (r *ConsentRepo) ActiveVersion(ctx context.Context, userID, clientID string) (version int64, ok bool, err error) {
	record, err := r.find(ctx, userID, clientID)
	if err != nil || record == nil || record.RevokedAt != nil {
		return 0, false, err
	}
	if record.Version <= 0 {
		return 1, true, nil
	}
	return record.Version, true, nil
}

// BumpUser 让该用户所有已签发的访问令牌失效，用于禁用账号这类需要全局撤销的动作。
func (r *ConsentRepo) BumpUser(ctx context.Context, userID string) error {
	err := r.db.WithContext(ctx).Model(&model.Consent{}).
		Where("user_id = ?", userID).
		Update("version", gorm.Expr("GREATEST(version, 1) + 1")).Error
	if err != nil {
		return fmt.Errorf("repo: 递增用户 %s 的授权版本: %w", userID, err)
	}
	return nil
}

func (r *ConsentRepo) TouchUsed(ctx context.Context, userID, clientID string, at time.Time) error {
	err := r.db.WithContext(ctx).Model(&model.Consent{}).
		Where("user_id = ? AND client_id = ? AND revoked_at IS NULL", userID, clientID).
		Update("last_used_at", at).Error
	if err != nil {
		return fmt.Errorf("repo: 更新授权使用时间 %s/%s: %w", userID, clientID, err)
	}
	return nil
}

func (r *ConsentRepo) ListActive(ctx context.Context, userID string) ([]Authorization, error) {
	var records []model.Consent
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Order("last_used_at DESC NULLS LAST, created_at DESC").
		Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 列出授权记录 %s: %w", userID, err)
	}
	if len(records) == 0 {
		return []Authorization{}, nil
	}

	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ClientID)
	}
	var clients []model.OAuthClient
	if err := r.db.WithContext(ctx).Where("client_id IN ?", ids).Find(&clients).Error; err != nil {
		return nil, fmt.Errorf("repo: 查询授权应用: %w", err)
	}
	known := make(map[string]model.OAuthClient, len(clients))
	for _, client := range clients {
		known[client.ClientID] = client
	}

	out := make([]Authorization, 0, len(records))
	for _, record := range records {
		client, ok := known[record.ClientID]
		if !ok {
			continue
		}
		out = append(out, Authorization{
			ClientID:    record.ClientID,
			Name:        client.Name,
			HomepageURL: client.HomepageURL,
			Scopes:      record.Scopes,
			GrantedAt:   record.CreatedAt,
			LastUsedAt:  record.LastUsedAt,
		})
	}
	return out, nil
}

func (r *ConsentRepo) find(ctx context.Context, userID, clientID string) (*model.Consent, error) {
	var record model.Consent
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND client_id = ?", userID, clientID).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo: 读取授权记录 %s/%s: %w", userID, clientID, err)
	}
	return &record, nil
}

func mergeScopes(existing, extra []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(extra))
	out := make([]string, 0, len(existing)+len(extra))
	for _, group := range [][]string{existing, extra} {
		for _, scope := range group {
			if _, ok := seen[scope]; ok {
				continue
			}
			seen[scope] = struct{}{}
			out = append(out, scope)
		}
	}
	return out
}
