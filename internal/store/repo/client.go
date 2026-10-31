package repo

import (
	"context"
	"encoding/json"
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
	ids := make([]uint64, 0)
	err := r.db.WithContext(ctx).Model(&model.GroupOAuthClient{}).
		Where("client_id = ?", clientID).
		Pluck("group_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询应用 %s 的用户组: %w", clientID, err)
	}
	return ids, nil
}

// Allows 判断用户能否授权该应用：非私有应用对登录用户全开，私有应用要求命中用户白名单或限定用户组。
func (r *ClientRepo) Allows(ctx context.Context, client *model.OAuthClient, userID string) (bool, error) {
	if !client.IsPrivate {
		return true, nil
	}

	var granted int64
	err := r.db.WithContext(ctx).Model(&model.ClientUserGrant{}).
		Where("client_id = ? AND user_id = ?", client.ClientID, userID).
		Count(&granted).Error
	if err != nil {
		return false, fmt.Errorf("repo: 查询应用 %s 的用户白名单: %w", client.ClientID, err)
	}
	if granted > 0 {
		return true, nil
	}

	groups, err := r.RestrictedGroups(ctx, client.ClientID)
	if err != nil {
		return false, err
	}
	if len(groups) == 0 {
		return false, nil
	}
	return r.UserInAnyGroup(ctx, userID, groups)
}

// UserInAnyGroup 判断用户是否命中给定组之一。用户直接所属的组与其所有祖先组都算命中，
// 组树因此可以在父组上配置一个大范围，在子组上细分。
func (r *ClientRepo) UserInAnyGroup(ctx context.Context, userID string, groupIDs []uint64) (bool, error) {
	if len(groupIDs) == 0 {
		return true, nil
	}

	const query = `
WITH RECURSIVE chain AS (
	SELECT g.id, g.parent_id
	FROM "groups" g
	JOIN user_groups ug ON ug.group_id = g.id
	WHERE ug.user_id = ?
	UNION
	SELECT parent.id, parent.parent_id
	FROM "groups" parent
	JOIN chain c ON parent.id = c.parent_id
)
SELECT COUNT(*) FROM chain WHERE id IN ?`

	var count int64
	err := r.db.WithContext(ctx).Raw(query, userID, groupIDs).Scan(&count).Error
	if err != nil {
		return false, fmt.Errorf("repo: 查询用户 %s 的用户组: %w", userID, err)
	}
	return count > 0, nil
}

func (r *ClientRepo) List(ctx context.Context) ([]model.OAuthClient, error) {
	var clients []model.OAuthClient
	if err := r.db.WithContext(ctx).Omit("logo").Order("name").Find(&clients).Error; err != nil {
		return nil, fmt.Errorf("repo: 列出应用: %w", err)
	}
	return clients, nil
}

func (r *ClientRepo) Create(ctx context.Context, client *model.OAuthClient) error {
	if err := r.db.WithContext(ctx).Create(client).Error; err != nil {
		return fmt.Errorf("repo: 创建应用 %s: %w", client.ClientID, err)
	}
	return nil
}

func (r *ClientRepo) Update(ctx context.Context, clientID string, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	// map 更新不经过字段的 serializer:json，[]string 需先序列化才能写入 jsonb 列。
	for key, value := range fields {
		values, ok := value.([]string)
		if !ok {
			continue
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return fmt.Errorf("repo: 序列化应用字段 %s: %w", key, err)
		}
		fields[key] = string(encoded)
	}
	err := r.db.WithContext(ctx).Model(&model.OAuthClient{}).
		Where("client_id = ?", clientID).
		Updates(fields).Error
	if err != nil {
		return fmt.Errorf("repo: 更新应用 %s: %w", clientID, err)
	}
	return nil
}

func (r *ClientRepo) Delete(ctx context.Context, clientID string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, target := range []any{
			&model.ClientAllowedScope{},
			&model.GroupOAuthClient{},
			&model.ClientUserGrant{},
			&model.Consent{},
			&model.LoginNotice{},
		} {
			if err := tx.Where("client_id = ?", clientID).Delete(target).Error; err != nil {
				return err
			}
		}
		return tx.Where("client_id = ?", clientID).Delete(&model.OAuthClient{}).Error
	})
	if err != nil {
		return fmt.Errorf("repo: 删除应用 %s: %w", clientID, err)
	}
	return nil
}

// ScopesOf 返回应用显式申请的 scope，不含全局默认项。
func (r *ClientRepo) ScopesOf(ctx context.Context, clientID string) ([]string, error) {
	names := make([]string, 0)
	err := r.db.WithContext(ctx).Model(&model.ClientAllowedScope{}).
		Where("client_id = ?", clientID).
		Order("scope_name").
		Pluck("scope_name", &names).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询应用 %s 的 scope: %w", clientID, err)
	}
	return names, nil
}

func (r *ClientRepo) SetScopes(ctx context.Context, clientID string, names []string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("client_id = ?", clientID).Delete(&model.ClientAllowedScope{}).Error; err != nil {
			return err
		}
		for _, name := range names {
			link := model.ClientAllowedScope{ClientID: clientID, ScopeName: name}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("repo: 设置应用 %s 的 scope: %w", clientID, err)
	}
	return nil
}

func (r *ClientRepo) GroupIDsOf(ctx context.Context, clientID string) ([]uint64, error) {
	ids := make([]uint64, 0)
	err := r.db.WithContext(ctx).Model(&model.GroupOAuthClient{}).
		Where("client_id = ?", clientID).
		Order("group_id").
		Pluck("group_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询应用 %s 的用户组: %w", clientID, err)
	}
	return ids, nil
}

func (r *ClientRepo) SetGroups(ctx context.Context, clientID string, groupIDs []uint64) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("client_id = ?", clientID).Delete(&model.GroupOAuthClient{}).Error; err != nil {
			return err
		}
		for _, groupID := range groupIDs {
			link := model.GroupOAuthClient{ClientID: clientID, GroupID: groupID}
			if err := tx.Create(&link).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("repo: 设置应用 %s 的用户组: %w", clientID, err)
	}
	return nil
}

func (r *ClientRepo) UsersOf(ctx context.Context, clientID string) ([]MemberView, error) {
	users := make([]MemberView, 0)
	err := r.db.WithContext(ctx).
		Table("client_user_grants").
		Select("users.id, users.name, users.email, users.status").
		Joins("JOIN users ON users.id = client_user_grants.user_id").
		Where("client_user_grants.client_id = ?", clientID).
		Order("users.id").
		Scan(&users).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询应用 %s 的用户白名单: %w", clientID, err)
	}
	return users, nil
}

func (r *ClientRepo) SetUsers(ctx context.Context, clientID string, userIDs []string) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("client_id = ?", clientID).Delete(&model.ClientUserGrant{}).Error; err != nil {
			return err
		}
		for _, userID := range userIDs {
			grant := model.ClientUserGrant{ClientID: clientID, UserID: userID}
			if err := tx.Create(&grant).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("repo: 设置应用 %s 的用户白名单: %w", clientID, err)
	}
	return nil
}
