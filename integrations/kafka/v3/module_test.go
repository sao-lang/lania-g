package kafka

import (
	stdctx "context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	mqadapter "github.com/sao-lang/lania-g/protocol/mq/v3"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"

	kgo "github.com/segmentio/kafka-go"
)

func TestForRoot_RegistersProvidersAndBindings(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{Brokers: []string{"127.0.0.1:9092"}})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	clientToken := reflect.TypeFor[*Client]()
	configPtrToken := reflect.TypeFor[*Config]()
	readerFactoryToken := reflect.TypeFor[ReaderFactory]()
	writerFactoryToken := reflect.TypeFor[WriterFactory]()
	factoryToken := reflect.TypeFor[Factory]()

	clientAny, err := mod.Container().Get(clientToken)
	if err != nil {
		t.Fatalf("get client: %v", err)
	}
	client := clientAny.(*Client)
	if len(client.Brokers()) != 1 || client.Brokers()[0] != "127.0.0.1:9092" {
		t.Fatalf("brokers=%v", client.Brokers())
	}

	configAny, err := mod.Container().Get(configPtrToken)
	if err != nil {
		t.Fatalf("get config ptr: %v", err)
	}
	if got := configAny.(*Config).Brokers[0]; got != "127.0.0.1:9092" {
		t.Fatalf("config brokers=%s", got)
	}

	if _, err := mod.Container().Get(readerFactoryToken); err != nil {
		t.Fatalf("get reader factory: %v", err)
	}
	if _, err := mod.Container().Get(writerFactoryToken); err != nil {
		t.Fatalf("get writer factory: %v", err)
	}
	if _, err := mod.Container().Get(factoryToken); err != nil {
		t.Fatalf("get factory: %v", err)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	value, err := br.Resolve(ctx, nil, clientToken, 0)
	if err != nil {
		t.Fatalf("resolve client binding: %v", err)
	}
	if value.(*Client) != client {
		t.Fatalf("resolved client mismatch")
	}

	factoryValue, err := br.Resolve(ctx, nil, factoryToken, 1)
	if err != nil {
		t.Fatalf("resolve factory binding: %v", err)
	}
	derived, err := factoryValue.(Factory).New(Config{Brokers: []string{"127.0.0.1:9092"}})
	if err != nil {
		t.Fatalf("factory new: %v", err)
	}
	if derived == nil || len(derived.Brokers()) != 1 {
		t.Fatalf("derived invalid")
	}
}

func TestClient_NewReaderAndWriterApplyDefaults(t *testing.T) {
	client, err := NewClient(Config{Brokers: []string{"127.0.0.1:9092"}})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	reader := client.NewReader(kgo.ReaderConfig{Topic: "topic-a"})
	if reader == nil {
		t.Fatalf("reader is nil")
	}
	_ = reader.Close()

	writer := client.NewWriter(kgo.WriterConfig{Topic: "topic-a"})
	if writer == nil {
		t.Fatalf("writer is nil")
	}
	_ = writer.Close()
}

type analyticsKafkaClient struct{}

func (analyticsKafkaClient) KafkaClientName() string { return "analytics" }

func TestBindings_ResolveNamedClientRef(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{Name: "analytics", Brokers: []string{"127.0.0.1:9092"}})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	refType := reflect.TypeOf(ClientRef[analyticsKafkaClient]{})
	handler := &runtime.Handler{
		Meta: &runtime.HandlerMeta{ParamPlans: []runtime.ParamPlan{{Index: 0, Type: refType}}},
	}
	value, err := br.Resolve(ctx, handler, refType, 0)
	if err != nil {
		t.Fatalf("resolve kafka ref: %v", err)
	}
	ref := value.(ClientRef[analyticsKafkaClient])
	if len(ref.Brokers()) != 1 || ref.Brokers()[0] != "127.0.0.1:9092" {
		t.Fatalf("brokers=%v", ref.Brokers())
	}
}

type fakeWriter struct {
	calls    atomic.Int32
	failures int32
	messages []kgo.Message
}

func (w *fakeWriter) WriteMessages(ctx stdctx.Context, messages ...kgo.Message) error {
	_ = ctx
	w.calls.Add(1)
	w.messages = append(w.messages, messages...)
	if w.failures > 0 {
		w.failures--
		return errors.New("write failed")
	}
	return nil
}

func (w *fakeWriter) Close() error { return nil }

type fakeReader struct {
	messages []kgo.Message
	index    int
}

func (r *fakeReader) FetchMessage(ctx stdctx.Context) (kgo.Message, error) {
	_ = ctx
	if r.index >= len(r.messages) {
		return kgo.Message{}, errors.New("eof")
	}
	msg := r.messages[r.index]
	r.index++
	return msg, nil
}

func (r *fakeReader) CommitMessages(ctx stdctx.Context, messages ...kgo.Message) error {
	_ = ctx
	_ = messages
	return nil
}

func (r *fakeReader) Close() error { return nil }

func TestProducer_RetryAndDLQ(t *testing.T) {
	writer := &fakeWriter{failures: 2}
	dlq := &fakeWriter{}
	producer := NewProducerWithWriter(ProducerConfig{
		RetryAttempts: 2,
		DLQTopic:      "topic-dlq",
	}, writer, dlq)
	err := producer.Publish(stdctx.Background(), kgo.Message{Topic: "topic-a", Value: []byte("payload")})
	if err == nil {
		t.Fatalf("expected publish error")
	}
	if writer.calls.Load() != 2 {
		t.Fatalf("writer calls=%d", writer.calls.Load())
	}
	if len(dlq.messages) != 1 {
		t.Fatalf("dlq messages=%d", len(dlq.messages))
	}
}

func TestConsumer_RetryAndDLQ(t *testing.T) {
	reader := &fakeReader{messages: []kgo.Message{{Topic: "topic-a", Value: []byte("payload")}}}
	dlq := &fakeWriter{}
	consumer := NewConsumerWithReader(ConsumerConfig{
		RetryAttempts: 2,
		DLQTopic:      "topic-dlq",
		AutoCommit:    true,
	}, reader, dlq)
	var calls atomic.Int32
	err := consumer.ConsumeOnce(stdctx.Background(), func(ctx stdctx.Context, msg Message) error {
		_ = ctx
		_ = msg
		calls.Add(1)
		return errors.New("consume failed")
	})
	if err == nil {
		t.Fatalf("expected consume error")
	}
	if calls.Load() != 2 {
		t.Fatalf("handler calls=%d", calls.Load())
	}
	if len(dlq.messages) != 1 {
		t.Fatalf("dlq messages=%d", len(dlq.messages))
	}
}

func TestProducer_PublishValueAndConsumerConfigFromMQ(t *testing.T) {
	writer := &fakeWriter{}
	producer := NewProducerWithWriter(ProducerConfig{RetryAttempts: 1}, writer, nil)
	err := producer.PublishValue(stdctx.Background(), map[string]string{"name": "demo"}, PublishOptions{
		Topic:   "topic-a",
		Key:     "key-1",
		Headers: map[string]string{"x-trace-id": "abc"},
	})
	if err != nil {
		t.Fatalf("publish value: %v", err)
	}
	if len(writer.messages) != 1 || writer.messages[0].Topic != "topic-a" {
		t.Fatalf("messages=%+v", writer.messages)
	}
	cfg := ConsumerConfigFromMQ(mqadapter.ConsumerConfig{Consumer: "kafka", Group: "group-a"}, "topic-a", ConsumeOptions{
		AutoCommit: true,
	})
	if cfg.GroupID != "group-a" || cfg.Topic != "topic-a" || !cfg.AutoCommit {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{Brokers: []string{"127.0.0.1:9092"}})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	if err := mod.Init(); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForRootCompat_InitRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRootCompat(Config{Brokers: []string{"127.0.0.1:9092"}})
	if err != nil {
		t.Fatalf("for root compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/kafka.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
