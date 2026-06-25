// dispatcher.go 实现 outbox 集成的消息派发与调度入口。
package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sao-lang/lania-g/integrations/events/v3"
)

// Dispatch 让 `DispatcherFunc` 适配 `Dispatcher` 接口。
func (f DispatcherFunc) Dispatch(ctx context.Context, message *Message) error { return f(ctx, message) }

// NewEventDispatcher 基于 events.Bus 创建一个 outbox 分发器。
func NewEventDispatcher(bus *events.Bus) Dispatcher {
	return DispatcherFunc(func(ctx context.Context, message *Message) error {
		if bus == nil {
			return fmt.Errorf("outbox dispatcher requires events bus")
		}
		return bus.Emit(ctx, message.Topic, message)
	})
}

// NewEmitterDispatcher 基于 events.Emitter 创建一个 outbox 分发器。
func NewEmitterDispatcher(emitter events.Emitter) Dispatcher {
	return DispatcherFunc(func(ctx context.Context, message *Message) error {
		if emitter == nil {
			return fmt.Errorf("outbox dispatcher requires events emitter")
		}
		return emitter.Emit(ctx, message.Topic, message)
	})
}

// MarshalPayload 把 payload 转换为可持久化的字节切片。
func MarshalPayload(payload any) ([]byte, error) {
	switch v := payload.(type) {
	case nil:
		return []byte("null"), nil
	case []byte:
		return append([]byte{}, v...), nil
	case string:
		return []byte(v), nil
	default:
		return json.Marshal(v)
	}
}
