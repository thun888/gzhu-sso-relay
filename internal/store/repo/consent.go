package repo

import (
	"context"
	"errors"
	"fmt"

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

func (r *ConsentRepo) Scopes(ctx context.Context, userID, clientID string) ([]string, error) {
	var scopes []string
	err := r.db.WithContext(ctx).Model(&model.Consent{}).
		Where("user_id = ? AND client_id = ?", userID, clientID).
		Pluck("scopes", &scopes).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo: 查询授权记录 %s/%s: %w", userID, clientID, err)
	}
	return scopes, nil
}

// Grant 写入或合并用户对应用的授权 scope。
func (r *ConsentRepo) Grant(ctx context.Context, userID, clientID string, scopes []string) error {
	existing, err := r.load(ctx, userID, clientID)
	if err != nil {
		return err
	}

	merged := mergeScopes(existing, scopes)
	record := model.Consent{UserID: userID, ClientID: clientID, Scopes: merged}

	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"scopes", "updated_at"}),
	}).Create(&record).Error
	if err != nil {
		return fmt.Errorf("repo: 写入授权记录 %s/%s: %w", userID, clientID, err)
	}
	return nil
}

func (r *ConsentRepo) Revoke(ctx context.Context, userID, clientID string) error {
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND client_id = ?", userID, clientID).
		Delete(&model.Consent{}).Error
	if err != nil {
		return fmt.Errorf("repo: 撤销授权记录 %s/%s: %w", userID, clientID, err)
	}
	return nil
}

func (r *ConsentRepo) load(ctx context.Context, userID, clientID string) ([]string, error) {
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
	return record.Scopes, nil
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
