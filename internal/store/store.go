package store

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/thun888/gzhu-sso-relay/internal/store/model"
)

func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Warn),
		TranslateError: true,
	})
	if err != nil {
		return nil, fmt.Errorf("store: 连接数据库: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("store: 获取连接池: %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

func Migrate(db *gorm.DB) error {
	hadPrivate := db.Migrator().HasColumn(&model.OAuthClient{}, "is_private")

	err := db.AutoMigrate(
		&model.User{},
		&model.Group{},
		&model.UserGroup{},
		&model.OAuthClient{},
		&model.GroupOAuthClient{},
		&model.Scope{},
		&model.ClientAllowedScope{},
		&model.OAuthAccount{},
		&model.Consent{},
		&model.ClientUserGrant{},
	)
	if err != nil {
		return fmt.Errorf("store: 迁移表结构: %w", err)
	}
	if err := migrateGroupSource(db); err != nil {
		return err
	}
	if !hadPrivate {
		if err := backfillClientPrivate(db); err != nil {
			return err
		}
	}
	return nil
}

// 应用早期只按用户组限制访问，加入 is_private 后把这些应用一次性标为私有，避免放开。
func backfillClientPrivate(db *gorm.DB) error {
	err := db.Exec(`UPDATE oauth_clients SET is_private = true
		WHERE EXISTS (SELECT 1 FROM group_oauth_clients WHERE group_oauth_clients.client_id = oauth_clients.client_id)`).Error
	if err != nil {
		return fmt.Errorf("store: 回填 oauth_clients.is_private: %w", err)
	}
	return nil
}

// 用户组来源早期存成 source 字符串，已并入 is_upstream 布尔值。
func migrateGroupSource(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasColumn(&model.Group{}, "source") {
		return nil
	}
	if err := db.Exec("UPDATE groups SET is_upstream = (source = 'upstream')").Error; err != nil {
		return fmt.Errorf("store: 回填 groups.is_upstream: %w", err)
	}
	if err := migrator.DropColumn(&model.Group{}, "source"); err != nil {
		return fmt.Errorf("store: 删除 groups.source: %w", err)
	}
	return nil
}

func Seed(db *gorm.DB) error {
	defaults := []model.Scope{
		{Name: "openid", DisplayName: "身份标识", Description: "返回用户唯一标识 sub", IsDefault: true},
		{Name: "profile", DisplayName: "基本资料", Description: "返回姓名、学号与班级", IsDefault: true},
		{Name: "email", DisplayName: "校内邮箱", Description: "返回自动生成的校内邮箱", IsDefault: true},
	}
	err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&defaults).Error
	if err != nil {
		return fmt.Errorf("store: 写入默认 scope: %w", err)
	}
	return nil
}
