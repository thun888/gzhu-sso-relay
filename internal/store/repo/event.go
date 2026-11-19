package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

type EventFilter struct {
	UserID   string
	ClientID string
	Kind     string
	Result   string
	From     *time.Time
	To       *time.Time
	Cursor   uint64
	Limit    int
}

type EventRepo struct {
	db *gorm.DB
}

func NewEventRepo(db *gorm.DB) *EventRepo {
	return &EventRepo{db: db}
}

func (r *EventRepo) Record(ctx context.Context, event *model.AuthEvent) error {
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	if err := r.db.WithContext(ctx).Create(event).Error; err != nil {
		return fmt.Errorf("repo: 写入事件 %s: %w", event.Kind, err)
	}
	return nil
}

func (r *EventRepo) List(ctx context.Context, filter EventFilter) ([]model.AuthEvent, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := r.db.WithContext(ctx).Model(&model.AuthEvent{}).Order("id DESC").Limit(limit)
	if filter.Cursor > 0 {
		query = query.Where("id < ?", filter.Cursor)
	}
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.ClientID != "" {
		query = query.Where("client_id = ?", filter.ClientID)
	}
	if filter.Kind != "" {
		query = query.Where("kind = ?", filter.Kind)
	}
	if filter.Result != "" {
		query = query.Where("result = ?", filter.Result)
	}
	if filter.From != nil {
		query = query.Where("occurred_at >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("occurred_at <= ?", *filter.To)
	}

	var events []model.AuthEvent
	if err := query.Find(&events).Error; err != nil {
		return nil, fmt.Errorf("repo: 查询事件: %w", err)
	}
	return events, nil
}

// AnonymizeUser 清除事件中的个人标识，保留事件本身用于统计。
func (r *EventRepo) AnonymizeUser(ctx context.Context, userID string) error {
	err := r.db.WithContext(ctx).Model(&model.AuthEvent{}).
		Where("user_id = ? OR actor_id = ?", userID, userID).
		Updates(map[string]any{"user_id": "", "actor_id": "", "ip": "", "user_agent": ""}).Error
	if err != nil {
		return fmt.Errorf("repo: 匿名化用户 %s 的事件: %w", userID, err)
	}
	return nil
}

func (r *EventRepo) Prune(ctx context.Context, before time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("occurred_at < ?", before).Delete(&model.AuthEvent{})
	if result.Error != nil {
		return 0, fmt.Errorf("repo: 清理事件: %w", result.Error)
	}
	return result.RowsAffected, nil
}
