package kafka_test

import . "github.com/sao-lang/lania-g/integrations/kafka/v3"
import (
	stdctx "context"
	"errors"
	"sync/atomic"
	"testing"

	mqadapter "github.com/sao-lang/lania-g/protocol/mq/v3"

	kgo "github.com/segmentio/kafka-go"
)

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
