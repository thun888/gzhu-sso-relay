package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// userInAnyGroup 判断用户是否命中给定组之一。用户直接所属的组与其所有祖先组都算命中，
// 组树因此可以在父组上配置一个大范围，在子组上细分。
func userInAnyGroup(db *gorm.DB, ctx context.Context, userID string, groupIDs []uint64) (bool, error) {
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
	if err := db.WithContext(ctx).Raw(query, userID, groupIDs).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("repo: 查询用户 %s 的用户组: %w", userID, err)
	}
	return count > 0, nil
}
