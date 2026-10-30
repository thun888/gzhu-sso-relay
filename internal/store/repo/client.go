package repo

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

type ClientRepo struct {
	db *gorm.DB
}

func NewClientRepo(db *gorm.DB) *ClientRepo {
	return &ClientRepo{db: db}
}

func (r *ClientRepo) ByClientID(ctx context.Context, clientID string) (*model.OAuthClient, error) {
	var c model.OAuthClient
	err := r.db.WithContext(ctx).Where("client_id = ?", clientID).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: 查询应用 %s: %w", clientID, err)
	}
	return &c, nil
}

// AllowedScopes 返回应用显式申请的 scope 与全局默认 scope 的并集。
func (r *ClientRepo) AllowedScopes(ctx context.Context, clientID string) ([]string, error) {
	var explicit []string
	err := r.db.WithContext(ctx).Model(&model.ClientAllowedScope{}).
		Where("client_id = ?", clientID).
		Pluck("scope_name", &explicit).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询应用 %s 允许的 scope: %w", clientID, err)
	}

	var defaults []string
	err = r.db.WithContext(ctx).Model(&model.Scope{}).
		Where("is_default = ?", true).
		Pluck("name", &defaults).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询默认 scope: %w", err)
	}

	seen := make(map[string]struct{}, len(explicit)+len(defaults))
	out := make([]string, 0, len(explicit)+len(defaults))
	for _, s := range append(explicit, defaults...) {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out, nil
}

// RestrictedGroups 返回允许访问该应用的用户组；为空表示不限制。
func (r *ClientRepo) RestrictedGroups(ctx context.Context, clientID string) ([]uint64, error) {
	var ids []uint64
	err := r.db.WithContext(ctx).Model(&model.GroupOAuthClient{}).
		Where("client_id = ?", clientID).
		Pluck("group_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询应用 %s 的用户组: %w", clientID, err)
	}
	return ids, nil
}

func (r *ClientRepo) UserInAnyGroup(ctx context.Context, userID string, groupIDs []uint64) (bool, error) {
	if len(groupIDs) == 0 {
		return true, nil
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&model.UserGroup{}).
		Where("user_id = ? AND group_id IN ?", userID, groupIDs).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("repo: 查询用户 %s 的用户组: %w", userID, err)
	}
	return count > 0, nil
}
