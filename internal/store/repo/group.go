package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

type GroupRepo struct {
	db *gorm.DB
}

func NewGroupRepo(db *gorm.DB) *GroupRepo {
	return &GroupRepo{db: db}
}

func (r *GroupRepo) List(ctx context.Context) ([]model.Group, error) {
	var groups []model.Group
	if err := r.db.WithContext(ctx).Order("id").Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("repo: 列出用户组: %w", err)
	}
	return groups, nil
}

func (r *GroupRepo) ByID(ctx context.Context, id uint64) (*model.Group, error) {
	var g model.Group
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("repo: 查询用户组 %d: %w", id, err)
	}
	return &g, nil
}

func (r *GroupRepo) Create(ctx context.Context, g *model.Group) error {
	if err := r.db.WithContext(ctx).Create(g).Error; err != nil {
		return fmt.Errorf("repo: 创建用户组 %s: %w", g.Code, err)
	}
	return nil
}

func (r *GroupRepo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	err := r.db.WithContext(ctx).Model(&model.Group{}).Where("id = ?", id).Updates(fields).Error
	if err != nil {
		return fmt.Errorf("repo: 更新用户组 %d: %w", id, err)
	}
	return nil
}

func (r *GroupRepo) Delete(ctx context.Context, id uint64) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", id).Delete(&model.UserGroup{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&model.GroupOAuthClient{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Group{}).Where("parent_id = ?", id).Update("parent_id", nil).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.Group{}).Error
	})
	if err != nil {
		return fmt.Errorf("repo: 删除用户组 %d: %w", id, err)
	}
	return nil
}

func (r *GroupRepo) Members(ctx context.Context, groupID uint64) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Model(&model.UserGroup{}).
		Where("group_id = ?", groupID).
		Order("user_id").
		Pluck("user_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 列出用户组 %d 的成员: %w", groupID, err)
	}
	return ids, nil
}

type MemberView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Status int16  `json:"status"`
}

func (r *GroupRepo) MemberViews(ctx context.Context, groupID uint64) ([]MemberView, error) {
	members := make([]MemberView, 0)
	err := r.db.WithContext(ctx).
		Table("user_groups").
		Select("users.id, users.name, users.email, users.status").
		Joins("JOIN users ON users.id = user_groups.user_id").
		Where("user_groups.group_id = ?", groupID).
		Order("users.id").
		Scan(&members).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 列出用户组 %d 的成员: %w", groupID, err)
	}
	return members, nil
}

func (r *GroupRepo) AddMember(ctx context.Context, groupID uint64, userID string) error {
	link := model.UserGroup{UserID: userID, GroupID: groupID}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error
	if err != nil {
		return fmt.Errorf("repo: 把 %s 加入用户组 %d: %w", userID, groupID, err)
	}
	return nil
}

func (r *GroupRepo) RemoveMember(ctx context.Context, groupID uint64, userID string) error {
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Delete(&model.UserGroup{}).Error
	if err != nil {
		return fmt.Errorf("repo: 把 %s 移出用户组 %d: %w", userID, groupID, err)
	}
	return nil
}

func (r *GroupRepo) OfUser(ctx context.Context, userID string) ([]model.Group, error) {
	var groups []model.Group
	err := r.db.WithContext(ctx).
		Joins("JOIN user_groups ON user_groups.group_id = groups.id").
		Where("user_groups.user_id = ?", userID).
		Order("groups.id").
		Find(&groups).Error
	if err != nil {
		return nil, fmt.Errorf("repo: 查询用户 %s 的用户组: %w", userID, err)
	}
	return groups, nil
}

// EffectiveCodes 返回用户的组标识，含直接所属的组与它们的全部祖先组，
// 与应用访问控制使用的判定范围保持一致。
func (r *GroupRepo) EffectiveCodes(ctx context.Context, userID string) ([]string, error) {
	const query = `
WITH RECURSIVE chain AS (
	SELECT g.id, g.parent_id, g.code
	FROM "groups" g
	JOIN user_groups ug ON ug.group_id = g.id
	WHERE ug.user_id = ?
	UNION
	SELECT parent.id, parent.parent_id, parent.code
	FROM "groups" parent
	JOIN chain c ON parent.id = c.parent_id
)
SELECT code FROM chain ORDER BY code`

	codes := make([]string, 0)
	if err := r.db.WithContext(ctx).Raw(query, userID).Scan(&codes).Error; err != nil {
		return nil, fmt.Errorf("repo: 查询用户 %s 的有效用户组: %w", userID, err)
	}
	return codes, nil
}

type GroupSpec struct {
	Code string
	Name string
	// ParentCode 为该组在本次同步中的上级组标识，父组变化时一并写回。
	ParentCode string
}

// SyncUpstream 用上游身份信息重建用户的同步组，手工维护的组不受影响。
// 组与成员关系都按差异写入：没有变化时不产生任何行更新。
func (r *GroupRepo) SyncUpstream(ctx context.Context, userID string, specs []GroupSpec) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(specs) == 0 {
			return tx.Exec(`DELETE FROM user_groups ug USING "groups" g
				WHERE ug.user_id = ? AND ug.group_id = g.id AND g.is_upstream`, userID).Error
		}

		values := make([]string, 0, len(specs))
		groupArgs := make([]any, 0, len(specs)*2)
		linkArgs := make([]any, 1, len(specs)+1)
		linkArgs[0] = userID
		placeholders := make([]string, 0, len(specs))
		for _, spec := range specs {
			values = append(values, "(?, ?, true, now(), now())")
			groupArgs = append(groupArgs, spec.Name, spec.Code)
			linkArgs = append(linkArgs, spec.Code)
			placeholders = append(placeholders, "?")
		}

		err := tx.Exec(`INSERT INTO "groups" (name, code, is_upstream, created_at, updated_at)
			VALUES `+strings.Join(values, ", ")+`
			ON CONFLICT (code) DO UPDATE SET
				name = EXCLUDED.name,
				is_upstream = EXCLUDED.is_upstream,
				updated_at = EXCLUDED.updated_at
			WHERE "groups".name IS DISTINCT FROM EXCLUDED.name
				OR "groups".is_upstream IS DISTINCT FROM EXCLUDED.is_upstream`, groupArgs...).Error
		if err != nil {
			return err
		}

		for _, spec := range specs {
			if spec.ParentCode == "" || spec.ParentCode == spec.Code {
				continue
			}
			err := tx.Exec(`UPDATE "groups" g SET parent_id = p.id, updated_at = now()
				FROM "groups" p
				WHERE g.code = ? AND p.code = ? AND g.parent_id IS DISTINCT FROM p.id`,
				spec.Code, spec.ParentCode).Error
			if err != nil {
				return err
			}
		}

		in := strings.Join(placeholders, ", ")
		err = tx.Exec(`DELETE FROM user_groups ug USING "groups" g
			WHERE ug.user_id = ? AND ug.group_id = g.id AND g.is_upstream AND g.code NOT IN (`+in+`)`,
			linkArgs...).Error
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO user_groups (user_id, group_id, created_at, updated_at)
			SELECT ?, g.id, now(), now() FROM "groups" g WHERE g.code IN (`+in+`)
			ON CONFLICT DO NOTHING`, linkArgs...).Error
	})
	if err != nil {
		return fmt.Errorf("repo: 同步用户 %s 的上游用户组: %w", userID, err)
	}
	return nil
}

// IsAncestorOf 判断 ancestorID 是否位于 id 的祖先链上，用来挡住成环的父组调整。
func (r *GroupRepo) IsAncestorOf(ctx context.Context, ancestorID, id uint64) (bool, error) {
	const query = `
WITH RECURSIVE chain AS (
	SELECT g.id, g.parent_id
	FROM "groups" g
	WHERE g.id = ?
	UNION
	SELECT parent.id, parent.parent_id
	FROM "groups" parent
	JOIN chain c ON parent.id = c.parent_id
)
SELECT COUNT(*) FROM chain WHERE id = ?`

	var count int64
	if err := r.db.WithContext(ctx).Raw(query, id, ancestorID).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("repo: 查询用户组 %d 的祖先链: %w", id, err)
	}
	return count > 0, nil
}
