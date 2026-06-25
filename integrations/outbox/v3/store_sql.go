// store_sql.go 提供 outbox 集成的 SQL 存储实现。
package outbox

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// SQLStore 是基于 GORM/SQL 的 Outbox Store 实现。
type SQLStore struct {
	db *gorm.DB
}

// NewSQLStore 基于 GORM 数据库连接创建一个 SQL Store。
func NewSQLStore(db *gorm.DB) Store {
	db.AutoMigrate(&Message{})
	return &SQLStore{db: db}
}

// Save 保存一条消息。
func (s *SQLStore) Save(ctx context.Context, msg *Message) error {
	return s.db.WithContext(ctx).Create(msg).Error
}

// Get 按 ID 读取一条消息。
func (s *SQLStore) Get(ctx context.Context, id string) (*Message, error) {
	var msg Message
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&msg).Error
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// ListPending 列出待处理或失败且达到可用时间的消息。
func (s *SQLStore) ListPending(ctx context.Context, limit int) ([]*Message, error) {
	var messages []*Message
	query := s.db.WithContext(ctx).Where(
		"status IN ? AND available_at <= ?",
		[]Status{StatusPending, StatusFailed},
		time.Now(),
	)

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&messages).Error
	return messages, err
}

// Mark 更新消息状态、错误信息与重试次数。
func (s *SQLStore) Mark(ctx context.Context, id string, status Status, lastError string, attempts int) error {
	return s.db.WithContext(ctx).Model(&Message{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     status,
		"last_error": lastError,
		"attempts":   attempts,
		"updated_at": time.Now(),
	}).Error
}
