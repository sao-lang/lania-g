// service.go 实现 outbox 集成的核心服务封装与业务入口。
package outbox

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	ormintegration "github.com/sao-lang/lania-g/integrations/orm/v3"
)

// Publish 创建一条待派发的 outbox 消息并写入 Store。
func (s *Service) Publish(ctx context.Context, topic string, payload any) (*Message, error) {
	if strings.TrimSpace(topic) == "" {
		return nil, fmt.Errorf("outbox topic is required")
	}
	data, err := MarshalPayload(payload)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	msg := &Message{
		ID:          uuid.NewString(),
		Topic:       topic,
		Payload:     data,
		Headers:     map[string]string{},
		Status:      StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
		AvailableAt: now,
	}
	if err := s.store.Save(ctx, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// PublishInTransaction 在事务上下文中创建一条 outbox 消息。
func (s *Service) PublishInTransaction(ctx context.Context, manager ormintegration.TransactionManager, topic string, payload any) (*Message, error) {
	if manager == nil {
		return s.Publish(ctx, topic, payload)
	}
	var message *Message
	err := manager.Do(ctx, func(txCtx context.Context, _ *gorm.DB) error {
		var err error
		message, err = s.Publish(txCtx, topic, payload)
		return err
	})
	if err != nil {
		return nil, err
	}
	return message, nil
}

// Flush 拉取待处理消息并并发派发。
func (s *Service) Flush(ctx context.Context, limit int) error {
	if s.dispatcher == nil {
		return fmt.Errorf("outbox dispatcher is not configured")
	}
	messages, err := s.store.ListPending(ctx, limit)
	if err != nil {
		return err
	}

	semaphore := make(chan struct{}, s.config.Scheduler.Concurrency)
	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	for _, msg := range messages {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(message *Message) {
			defer func() {
				wg.Done()
				<-semaphore
			}()

			if err := s.dispatchMessage(ctx, message); err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}(msg)
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (s *Service) dispatchMessage(ctx context.Context, msg *Message) error {
	if err := s.dispatcher.Dispatch(ctx, msg); err != nil {
		attempts := msg.Attempts + 1
		status := StatusFailed
		if attempts >= s.config.MaxAttempts {
			status = StatusDead
			if s.deadLetter != nil {
				if deadLetterErr := s.deadLetter.Dispatch(ctx, msg); deadLetterErr != nil {
					fmt.Printf("Dead letter dispatch failed: %v\n", deadLetterErr)
				}
			}
		}
		if markErr := s.store.Mark(ctx, msg.ID, status, err.Error(), attempts); markErr != nil {
			return markErr
		}
		return nil
	}
	if err := s.store.Mark(ctx, msg.ID, StatusDispatched, "", msg.Attempts+1); err != nil {
		return err
	}
	return nil
}

// ReprocessDead 重新处理死信消息。
// 当前实现保留接口，尚未提供具体逻辑。
func (s *Service) ReprocessDead(ctx context.Context, limit int) error {
	return nil
}

// CleanupOldMessages 清理过旧消息。
// 当前实现保留接口，尚未提供具体逻辑。
func (s *Service) CleanupOldMessages(ctx context.Context, olderThan time.Duration) error {
	return nil
}

// Receive 在 Inbox 去重校验通过后执行消息处理函数。
func (s *Service) Receive(ctx context.Context, message *Message, handler func(context.Context, *Message) error) error {
	if message == nil || handler == nil {
		return fmt.Errorf("outbox inbox requires message and handler")
	}
	if !s.inbox.Allow(message.ID) {
		return nil
	}
	return handler(ctx, message)
}

// Allow 判断消息是否允许被处理；同一 ID 只会放行一次。
func (i *Inbox) Allow(id string) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if _, ok := i.processed[id]; ok {
		return false
	}
	i.processed[id] = time.Now()
	return true
}
