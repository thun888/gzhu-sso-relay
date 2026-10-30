package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

type ScopeRepo struct {
	db *gorm.DB
}

func NewScopeRepo(db *gorm.DB) *ScopeRepo {
	return &ScopeRepo{db: db}
}

func (r *ScopeRepo) ByNames(ctx context.Context, names []string) ([]model.Scope, error) {
	if len(names) == 0 {
		return nil, nil
	}

	var scopes []model.Scope
	if err := r.db.WithContext(ctx).Where("name IN ?", names).Find(&scopes).Error; err != nil {
		return nil, fmt.Errorf("repo: 查询 scope 详情: %w", err)
	}
	return scopes, nil
}

func (r *ScopeRepo) List(ctx context.Context) ([]model.Scope, error) {
	var scopes []model.Scope
	if err := r.db.WithContext(ctx).Order("is_default DESC, name").Find(&scopes).Error; err != nil {
		return nil, fmt.Errorf("repo: 列出 scope: %w", err)
	}
	return scopes, nil
}
