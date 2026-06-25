package kafka

import (
	stdctx "context"
	"errors"
	"sync/atomic"
	"testing"

	mqadapter "github.com/sao-lang/lania-g/protocol/mq/v3"

	kgo "github.com/segmentio/kafka-go"
)

type stubReader struct {
	msgs        []kgo.Message
	fetchErr    error
	commitCount atomic.Int32
	closeCount  atomic.Int32
}

func (r *stubReader) FetchMessage(ctx stdctx.Context) (kgo.Message, error) {
	if r.fetchErr != nil {
		return kgo.Message{}, r.fetchErr
	}
	if len(r.msgs) == 0 {
		return kgo.Message{}, stdctx.Canceled
	}
	msg := r.msgs[0]
	r.msgs = r.msgs[1:]
	return msg, nil
}

func (r *stubReader) CommitMessages(ctx stdctx.Context, msgs ...kgo.Message) error {
	r.commitCount.Add(int32(len(msgs)))
	return nil
}

func (r *stubReader) Close() error {
	r.closeCount.Add(1)
	return nil
}

func TestDriverOpen_SingleTopic(t *testing.T) {
	var got kgo.ReaderConfig
	d := New(Config{Brokers: []string{"127.0.0.1:9092"}})
	d.newReader = func(rc kgo.ReaderConfig) reader {
		got = rc
		return &stubReader{}
	}

	sess, err := d.Open(stdctx.Background(), mqadapter.ConsumerConfig{
		Consumer: "user-consumer",
		Group:    "g1",
		Topics:   []string{"user.created"},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()

	if got.Topic != "user.created" {
		t.Fatalf("topic=%q", got.Topic)
	}
	if got.GroupID != "g1" {
		t.Fatalf("group=%q", got.GroupID)
	}
	if len(got.GroupTopics) != 0 {
		t.Fatalf("group topics=%v", got.GroupTopics)
	}
}

func TestDriverOpen_MultiTopic(t *testing.T) {
	var got kgo.ReaderConfig
	d := New(Config{Brokers: []string{"127.0.0.1:9092"}, GroupID: "default-group"})
	d.newReader = func(rc kgo.ReaderConfig) reader {
		got = rc
		return &stubReader{}
	}

	sess, err := d.Open(stdctx.Background(), mqadapter.ConsumerConfig{
		Consumer: "user-consumer",
		Topics:   []string{"user.created", "user.updated"},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer sess.Close()

	if got.GroupID != "default-group" {
		t.Fatalf("group=%q", got.GroupID)
	}
	if got.Topic != "" {
		t.Fatalf("topic=%q", got.Topic)
	}
	if len(got.GroupTopics) != 2 {
		t.Fatalf("group topics=%v", got.GroupTopics)
	}
}

func TestSessionPoll_MapsMessageAndAck(t *testing.T) {
	r := &stubReader{
		msgs: []kgo.Message{{
			Topic: "user.created",
			Key:   []byte("u1"),
			Value: []byte(`{"id":"u1"}`),
			Headers: []kgo.Header{
				{Key: "X-Trace-Id", Value: []byte("trace-1")},
				{Key: "X-Trace-Id", Value: []byte("trace-2")},
			},
		}},
	}
	sess := &session{reader: r}

	msg, err := sess.Poll(stdctx.Background())
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if msg.Topic != "user.created" || msg.Key != "u1" {
		t.Fatalf("msg=%+v", msg)
	}
	if got := len(msg.Headers["X-Trace-Id"]); got != 2 {
		t.Fatalf("headers=%v", msg.Headers)
	}
	if string(msg.Body) != `{"id":"u1"}` {
		t.Fatalf("body=%s", string(msg.Body))
	}

	if err := msg.Ack(); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if err := msg.Ack(); err != nil {
		t.Fatalf("ack second: %v", err)
	}
	if got := r.commitCount.Load(); got != 1 {
		t.Fatalf("commit count=%d", got)
	}
}

func TestDriverOpen_ValidateConfig(t *testing.T) {
	d := New(Config{})
	if _, err := d.Open(stdctx.Background(), mqadapter.ConsumerConfig{Consumer: "c1", Topics: []string{"t1"}}); err == nil {
		t.Fatalf("want brokers error")
	}
	d = New(Config{Brokers: []string{"127.0.0.1:9092"}})
	if _, err := d.Open(stdctx.Background(), mqadapter.ConsumerConfig{Consumer: "c1"}); err == nil {
		t.Fatalf("want topics error")
	}
}

func TestSessionPoll_PropagatesFetchError(t *testing.T) {
	want := errors.New("fetch failed")
	sess := &session{reader: &stubReader{fetchErr: want}}
	_, err := sess.Poll(stdctx.Background())
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
}
