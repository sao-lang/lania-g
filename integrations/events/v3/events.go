// events.go 定义 events 集成的事件总线、事件对象与分发基础能力。
package events

import (
	stdctx "context"
	"sync"
)

// Handler 表示事件处理函数签名。
type Handler func(stdctx.Context, ...interface{}) error

// Config 描述事件总线的初始化配置。
type Config struct {
	Async bool
}

// Emitter 定义事件发射器需要实现的最小能力。
type Emitter interface {
	Emit(stdctx.Context, string, ...interface{}) error
	EmitAsync(stdctx.Context, string, ...interface{})
}

// Factory 定义事件总线工厂接口。
type Factory interface {
	Default() *Bus
	New(cfg Config) (*Bus, error)
}

// Bus 是一个轻量的进程内事件总线。
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]*subscription
	cfg      Config
}

type subscription struct {
	once    bool
	handler Handler
}

// New 基于配置创建一个事件总线。
func New(cfg Config) (*Bus, error) {
	return &Bus{
		handlers: make(map[string][]*subscription),
		cfg:      cfg,
	}, nil
}

// Default 返回当前总线本身，用于满足 Factory 接口。
func (b *Bus) Default() *Bus { return b }

// New 基于 cfg 创建一个新的事件总线，用于满足 Factory 接口。
func (b *Bus) New(cfg Config) (*Bus, error) { return New(cfg) }

// Config 返回当前总线的配置快照。
func (b *Bus) Config() Config { return b.cfg }

// On 为指定事件注册一个常驻处理函数。
func (b *Bus) On(event string, handler Handler) {
	if handler == nil || event == "" {
		return
	}
	b.mu.Lock()
	b.handlers[event] = append(b.handlers[event], &subscription{handler: handler})
	b.mu.Unlock()
}

// Once 为指定事件注册一个只执行一次的处理函数。
func (b *Bus) Once(event string, handler Handler) {
	if handler == nil || event == "" {
		return
	}
	b.mu.Lock()
	b.handlers[event] = append(b.handlers[event], &subscription{once: true, handler: handler})
	b.mu.Unlock()
}

// Off 移除某个事件下的全部处理函数。
func (b *Bus) Off(event string) {
	b.mu.Lock()
	delete(b.handlers, event)
	b.mu.Unlock()
}

// Emit 同步触发一个事件，并按注册顺序执行处理函数。
func (b *Bus) Emit(ctx stdctx.Context, event string, args ...interface{}) error {
	subs := b.snapshot(event)
	if len(subs) == 0 {
		return nil
	}
	for _, sub := range subs {
		if err := sub.handler(ctx, args...); err != nil {
			return err
		}
	}
	b.pruneOnce(event)
	return nil
}

// EmitAsync 异步触发一个事件。
func (b *Bus) EmitAsync(ctx stdctx.Context, event string, args ...interface{}) {
	go func() { _ = b.Emit(ctx, event, args...) }()
}

func (b *Bus) snapshot(event string) []*subscription {
	b.mu.RLock()
	defer b.mu.RUnlock()
	subs := b.handlers[event]
	out := make([]*subscription, len(subs))
	copy(out, subs)
	return out
}

func (b *Bus) pruneOnce(event string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.handlers[event]
	if len(subs) == 0 {
		return
	}
	keep := subs[:0]
	for _, sub := range subs {
		if !sub.once {
			keep = append(keep, sub)
		}
	}
	if len(keep) == 0 {
		delete(b.handlers, event)
		return
	}
	b.handlers[event] = keep
}
