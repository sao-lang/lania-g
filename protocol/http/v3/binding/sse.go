// sse.go 提供 HTTP SSE 响应写出与流式发送辅助。
package http

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// SSEEvent 表示一条 Server-Sent Events 消息。
type SSEEvent struct {
	Event string
	Data  string
	ID    string
	Retry int
}

// String 将 SSEEvent 按标准 SSE 文本格式序列化。
func (e SSEEvent) String() string {
	var sb strings.Builder
	if e.ID != "" {
		sb.WriteString(fmt.Sprintf("id: %s\n", e.ID))
	}
	if e.Event != "" {
		sb.WriteString(fmt.Sprintf("event: %s\n", e.Event))
	}
	if e.Retry > 0 {
		sb.WriteString(fmt.Sprintf("retry: %d\n", e.Retry))
	}
	if e.Data != "" {
		for _, line := range strings.Split(e.Data, "\n") {
			sb.WriteString(fmt.Sprintf("data: %s\n", line))
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// SSEStream 表示一个 SSE 写入流。
type SSEStream interface {
	Write(event SSEEvent) error
	Close() error
	Done() <-chan struct{}
}

type sseStream struct {
	w       io.Writer
	flusher http.Flusher
	done    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	closed  bool
}

// NewSSEStream 初始化 SSE 必要 Header 并返回一个写入流。
// 它只负责传输层输出，不触碰 runtime context。
func NewSSEStream(w http.ResponseWriter) SSEStream {
	flusher, _ := w.(http.Flusher)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// 沿用旧行为，默认允许跨域消费 SSE；若有更严格策略，可由 adapter/header 再覆盖。
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	return &sseStream{
		w:       w,
		flusher: flusher,
		done:    make(chan struct{}),
	}
}

// NewSSEStreamFromContext 是面向可注入 `http.Context` 的便捷封装。
// 它也会将响应标记为已写入，以便内置 HTTP adapter 正确处理响应流程。
func NewSSEStreamFromContext(ctx Context) SSEStream {
	if hc, ok := ctx.(*HttpContext); ok && hc != nil {
		// SSE 一旦开始写流，就不应再让 adapter 尝试走默认 JSON/文本响应出口。
		hc.markWritten()
		hc.aborted = true
	}
	return NewSSEStream(ctx.Writer())
}

// Write 向客户端推送一条 SSE 消息。
func (s *sseStream) Write(event SSEEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("stream closed")
	}

	_, err := s.w.Write([]byte(event.String()))
	if err != nil {
		return err
	}
	if s.flusher != nil {
		// SSE 需要每条事件尽快 flush 给客户端，否则会被缓冲层吞掉“流式”效果。
		s.flusher.Flush()
	}
	return nil
}

// Close 关闭流并通知 Done。
func (s *sseStream) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.done)
	})
	return nil
}

// Done 返回在 stream 关闭时被关闭的通道。
func (s *sseStream) Done() <-chan struct{} { return s.done }
