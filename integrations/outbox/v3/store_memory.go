// store_memory.go 提供 outbox 集成的内存存储实现。
package outbox

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type memoryStore struct {
	mu       sync.RWMutex
	messages map[string]*Message
}

// NewMemoryStore 创建一个基于内存的 Outbox Store。
func NewMemoryStore() Store {
	return &memoryStore{messages: map[string]*Message{}}
}

// Save 保存一条消息。
func (m *memoryStore) Save(ctx context.Context, msg *Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[msg.ID] = cloneMessage(msg)
	return nil
}

// Get 按 ID 读取一条消息。
func (m *memoryStore) Get(ctx context.Context, id string) (*Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msg, ok := m.messages[id]
	if !ok {
		return nil, fmt.Errorf("outbox message not found")
	}
	return cloneMessage(msg), nil
}

// ListPending 列出待处理或失败且达到可用时间的消息。
func (m *memoryStore) ListPending(ctx context.Context, limit int) ([]*Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := make([]*Message, 0)
	now := time.Now()
	for _, msg := range m.messages {
		if msg.Status == StatusPending || msg.Status == StatusFailed {
			if msg.AvailableAt.After(now) {
				continue
			}
			items = append(items, cloneMessage(msg))
		}
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	return items, nil
}

// Mark 更新消息状态、错误信息与重试次数。
func (m *memoryStore) Mark(ctx context.Context, id string, status Status, lastError string, attempts int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg, ok := m.messages[id]
	if !ok {
		return fmt.Errorf("outbox message not found")
	}
	msg.Status = status
	msg.Attempts = attempts
	msg.LastError = lastError
	msg.UpdatedAt = time.Now()
	return nil
}

func cloneMessage(in *Message) *Message {
	if in == nil {
		return nil
	}
	out := *in
	out.Payload = append([]byte{}, in.Payload...)
	out.Headers = map[string]string{}
	for k, v := range in.Headers {
		out.Headers[k] = v
	}
	return &out
}
