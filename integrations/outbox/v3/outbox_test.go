package outbox

import (
	"context"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/integrations/events/v3"
	ormintegration "github.com/sao-lang/lania-g/integrations/orm/v3"
)

func TestPublishFlushAndReceive(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	bus, err := events.New(events.Config{})
	if err != nil {
		t.Fatalf("new bus: %v", err)
	}
	var delivered atomic.Int32
	bus.On("user.created", func(ctx context.Context, args ...any) error {
		delivered.Add(1)
		if len(args) == 0 {
			t.Fatalf("args empty")
		}
		return nil
	})

	service, err := New(Config{
		Dispatcher: NewEventDispatcher(bus),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	msg, err := service.Publish(context.Background(), "user.created", map[string]any{"id": "u1"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := service.Flush(context.Background(), 10); err != nil {
		t.Fatalf("flush: %v", err)
	}
	stored, err := service.store.Get(context.Background(), msg.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Status != StatusDispatched {
		t.Fatalf("status = %s", stored.Status)
	}
	if delivered.Load() != 1 {
		t.Fatalf("delivered=%d", delivered.Load())
	}

	var consumed atomic.Int32
	if err := service.Receive(context.Background(), stored, func(ctx context.Context, message *Message) error {
		consumed.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("receive first: %v", err)
	}
	if err := service.Receive(context.Background(), stored, func(ctx context.Context, message *Message) error {
		consumed.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("receive second: %v", err)
	}
	if consumed.Load() != 1 {
		t.Fatalf("consumed=%d", consumed.Load())
	}
}

func TestDeadLetterAfterMaxAttempts(t *testing.T) {
	var dead atomic.Int32
	service, err := New(Config{
		MaxAttempts: 1,
		Dispatcher: DispatcherFunc(func(ctx context.Context, message *Message) error {
			return context.Canceled
		}),
		DeadLetter: DispatcherFunc(func(ctx context.Context, message *Message) error {
			dead.Add(1)
			return nil
		}),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	msg, err := service.Publish(context.Background(), "dead.event", "payload")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := service.Flush(context.Background(), 10); err != nil {
		t.Fatalf("flush: %v", err)
	}
	stored, err := service.store.Get(context.Background(), msg.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Status != StatusDead {
		t.Fatalf("status = %s", stored.Status)
	}
	if dead.Load() != 1 {
		t.Fatalf("dead letters=%d", dead.Load())
	}
}

func TestPublishInTransaction(t *testing.T) {
	service, err := New(Config{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	manager := fakeTxManager{}
	msg, err := service.PublishInTransaction(context.Background(), manager, "tx.event", map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("publish in transaction: %v", err)
	}
	if msg == nil || msg.Topic != "tx.event" {
		t.Fatalf("message = %+v", msg)
	}
}

type fakeTxManager struct{}

func (fakeTxManager) Do(ctx context.Context, fn func(context.Context, *gorm.DB) error) error {
	return fn(ctx, nil)
}

func (fakeTxManager) Current(ctx context.Context) *gorm.DB { return nil }

func (fakeTxManager) With(ctx context.Context, db *gorm.DB) context.Context { return ctx }

var _ ormintegration.TransactionManager = fakeTxManager{}
