package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

type NoticeRepo struct {
	db *gorm.DB
}

func NewNoticeRepo(db *gorm.DB) *NoticeRepo {
	return &NoticeRepo{db: db}
}

// MarkFirst 抢占式写入通知记录，返回是否为该组合的第一次，并发授权下也只有一个调用方拿到 true。
func (r *NoticeRepo) MarkFirst(ctx context.Context, userID, clientID string) (bool, error) {
	record := model.LoginNotice{UserID: userID, ClientID: clientID}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return false, fmt.Errorf("repo: 写入登录通知记录 %s/%s: %w", userID, clientID, result.Error)
	}
	return result.RowsAffected == 1, nil
}
