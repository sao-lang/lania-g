package mq

import (
	stdctx "context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mqbinding "github.com/sao-lang/lania-g/protocol/mq/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	mqprotocol "github.com/sao-lang/lania-g/protocol/mq/v3/protocol"
)

type testConsumer struct {
	mu       sync.Mutex
	received []testPayload
}

type testPayload struct {
	ID string `json:"id"`
}

type mqOwnerModuleA struct{ *module.BaseModule }
type mqOwnerModuleB struct{ *module.BaseModule }

var observedPayload chan testPayload

func (c *testConsumer) Handle(
	ctx stdctx.Context,
	payload testPayload,
	headers mqbinding.Headers,
	trace mqbinding.Header[string],
	topic mqbinding.Topic,
	ack mqbinding.Ack,
) error {
	if ctx == nil {
		return stdctx.Canceled
	}
	if string(topic) != "user.created" {
		return stdctx.Canceled
	}
	if trace.Value != "trace-1" {
		return stdctx.Canceled
	}
	if headers["x-trace-id"][0] != "trace-1" {
		return stdctx.Canceled
	}
	c.mu.Lock()
	c.received = append(c.received, payload)
	c.mu.Unlock()
	if observedPayload != nil {
		select {
		case observedPayload <- payload:
		default:
		}
	}
	return ack()
}

type memoryDriver struct {
	ch     chan *Message
	polled chan struct{}
	opened atomic.Int32
}

type memorySession struct {
	ch     chan *Message
	polled chan struct{}
}

func TestPluginScan_RequiresExplicitRegistry(t *testing.T) {
	consumer := &testConsumer{}
	pConsumer, _ := di.ProviderFromInstance(consumer, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pConsumer}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Consumer("default", consumer).On("user.created", consumer.Handle).Build()

	_, err := (&Plugin{}).Scan(moduleRef, nil)
	if err == nil {
		t.Fatalf("expected missing registry error")
	}
	if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile with explicit registry: %v", err)
	}
	if compiled == nil {
		t.Fatalf("expected compiled app")
	}
}

func (d *memoryDriver) Open(ctx stdctx.Context, cfg ConsumerConfig) (Session, error) {
	d.opened.Add(1)
	return &memorySession{ch: d.ch, polled: d.polled}, nil
}

func (s *memorySession) Poll(ctx stdctx.Context) (*Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-s.ch:
		if s.polled != nil {
			select {
			case s.polled <- struct{}{}:
			default:
			}
		}
		return msg, nil
	}
}

func (s *memorySession) Close() error { return nil }

func TestMQAdapter_CompileAndConsume(t *testing.T) {
	consumer := &testConsumer{}
	pConsumer, _ := di.ProviderFromInstance(consumer, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pConsumer}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	observedPayload = make(chan testPayload, 2)
	defer func() { observedPayload = nil }()
	NewAPI(reg).
		Consumer("default", consumer).
		Group("g1").
		On("user.created", consumer.Handle).
		BindParam(3, "x-trace-id").
		Build()

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil {
		t.Fatalf("install: %v", err)
	}

	driver := &memoryDriver{ch: make(chan *Message, 1), polled: make(chan struct{}, 1)}
	adapter := New(driver)
	host := &testHost{rt: rt, reg: reg, moduleRef: moduleRef}
	if got := len(collectConsumerConfigs(reg)); got != 1 {
		t.Fatalf("consumer configs=%d", got)
	}
	if err := adapter.Mount(host); err != nil {
		t.Fatalf("mount: %v", err)
	}
	if err := adapter.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer adapter.Stop()

	var acked atomic.Int32
	ackCh := make(chan struct{}, 2)
	errCh := make(chan error, 1)
	driver.ch <- &Message{
		Topic:   "user.created",
		Headers: map[string][]string{"X-Trace-Id": {"trace-1"}},
		Body:    []byte(`{"id":"u1"}`),
		Ack: func() error {
			acked.Add(1)
			select {
			case ackCh <- struct{}{}:
			default:
			}
			return nil
		},
		Nack: func(err error) error {
			select {
			case errCh <- err:
			default:
			}
			return nil
		},
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting message: opened=%d acked=%d", driver.opened.Load(), acked.Load())
		case err := <-errCh:
			t.Fatalf("nack: %v", err)
		case payload := <-observedPayload:
			if payload.ID != "u1" {
				t.Fatalf("payload id=%s", payload.ID)
			}
			if acked.Load() >= 1 {
				return
			}
		case <-driver.polled:
		case <-ackCh:
			if acked.Load() >= 1 {
				return
			}
		default:
			if acked.Load() >= 1 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

type testHost struct {
	rt        *runtime.Runtime
	reg       *registry.Registry
	moduleRef *module.ModuleRef
}

func (h *testHost) Runtime() *runtime.Runtime    { return h.rt }
func (h *testHost) Registry() *registry.Registry { return h.reg }
func (h *testHost) ModuleRef() *module.ModuleRef { return h.moduleRef }

func TestMQBinding_RouteKey(t *testing.T) {
	key := runtime.BuildRouteKey(mqprotocol.Protocol, "user.created", "default")
	if key != "mq:user.created:default" {
		t.Fatalf("route key=%s", key)
	}
}

func TestMQRuntime_Execute(t *testing.T) {
	consumer := &testConsumer{}
	pConsumer, _ := di.ProviderFromInstance(consumer, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pConsumer}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).
		Consumer("default", consumer).
		Group("g1").
		On("user.created", consumer.Handle).
		BindParam(3, "x-trace-id").
		Build()

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil {
		t.Fatalf("install: %v", err)
	}

	ctx := runtime.NewHandlerContext(mqprotocol.Protocol)
	ctx.WithContext(stdctx.Background())
	ctx.Request.Method = "user.created"
	ctx.Request.Path = "default"
	ctx.Request.BodyBytes = []byte(`{"id":"u1"}`)
	ctx.Set(mqbinding.MetadataKeyHeaders, map[string][]string{"x-trace-id": {"trace-1"}})
	ctx.Set(mqbinding.MetadataKeyTopic, "user.created")
	ctx.Set(mqbinding.MetadataKeyConsumer, "default")
	ctx.Set(mqbinding.MetadataKeyAck, func() error { return nil })

	if _, err := rt.Execute(ctx); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestMQPlugin_OwnerErrorsUseUnifiedMeta(t *testing.T) {
	providerA, _ := di.ProviderFromInstance(&testConsumer{}, di.Singleton)
	providerB, _ := di.ProviderFromInstance(&testConsumer{}, di.Singleton)
	modA := &mqOwnerModuleA{BaseModule: module.NewBaseModule(&module.ModuleMetadata{Providers: []*di.Provider{providerA}})}
	modB := &mqOwnerModuleB{BaseModule: module.NewBaseModule(&module.ModuleMetadata{Providers: []*di.Provider{providerB}})}
	root := module.CreateModule([]module.Module{modA, modB}, nil, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}

	reg := registry.New()
	receiver := &testConsumer{}
	NewAPI(reg).Consumer("default", receiver).On("user.created", receiver.Handle).Build()

	_, err := compiler.Compile(module.NewModuleRef(root), reg, NewPlugin())
	if err == nil {
		t.Fatalf("expected owner ambiguity error")
	}

	var kernelErr *kerrors.KernelError
	if !errors.As(err, &kernelErr) {
		t.Fatalf("expected KernelError, got %T", err)
	}
	if got := kernelErr.Meta["ownerKind"]; got != "receiver" {
		t.Fatalf("ownerKind = %v, want receiver", got)
	}
	if got := kernelErr.Meta["ownerStatus"]; got != "ambiguous" {
		t.Fatalf("ownerStatus = %v, want ambiguous", got)
	}
	if got := kernelErr.Meta["ownerToken"]; got == "" {
		t.Fatalf("ownerToken = %v, want non-empty", got)
	}
	candidates, ok := kernelErr.Meta["ownerCandidates"].([]string)
	if !ok || len(candidates) != 2 {
		t.Fatalf("ownerCandidates = %#v, want 2 entries", kernelErr.Meta["ownerCandidates"])
	}
}
