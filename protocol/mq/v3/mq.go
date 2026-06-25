// mq.go 实现 MQ adapter 的主入口与宿主集成逻辑。
package mq

import (
	stdctx "context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	mqbinding "github.com/sao-lang/lania-g/protocol/mq/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	mqprotocol "github.com/sao-lang/lania-g/protocol/mq/v3/protocol"
)

// Adapter 是 MQ 协议的运行期适配器，负责打开 consumer session 并消费消息。
type Adapter struct {
	host adapter.Host
	api  *API

	driver Driver

	mu       sync.Mutex
	cancel   stdctx.CancelFunc
	sessions []Session
	started  bool
}

// New 创建 MQ adapter，并注入一个底层驱动实现。
//
// MQ adapter 本身不直接决定如何连接具体消息系统；
// 连接、轮询、ack/nack 等行为由传入的 Driver 负责。
func New(driver Driver) *Adapter {
	return &Adapter{
		api:      NewCompatAPI(),
		driver:   driver,
		sessions: make([]Session, 0),
	}
}

// ID 返回该 adapter 的唯一标识。
func (a *Adapter) ID() string { return AdapterID }

// Plugins 返回 MQ 协议参与编译的插件列表。
func (a *Adapter) Plugins() []compiler.ProtocolPlugin { return []compiler.ProtocolPlugin{NewPlugin()} }

// API 返回 MQ adapter 暴露给应用侧的 DSL API。
func (a *Adapter) API() any { return a.api }

// Mount 将 MQ adapter 挂到应用 host 上，并把 API 绑定到当前 registry。
func (a *Adapter) Mount(host adapter.Host) error {
	if host == nil {
		return fmt.Errorf("mq adapter host is nil")
	}
	a.host = host
	a.api = NewAPI(host.Registry())
	return nil
}

// Start 启动 MQ adapter。
//
// 启动流程大致为：
// - 从 registry 收集消费者与订阅声明
// - 通过 Driver 为每个 consumer 打开 session
// - 后台轮询消息并转发到 runtime.Execute
func (a *Adapter) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return nil
	}
	if a.host == nil {
		return fmt.Errorf("mq adapter not mounted")
	}
	if a.driver == nil {
		return fmt.Errorf("mq adapter driver is nil")
	}

	consumers := collectConsumerConfigs(a.host.Registry())
	if len(consumers) == 0 {
		a.started = true
		return nil
	}

	ctx, cancel := stdctx.WithCancel(stdctx.Background())
	a.cancel = cancel
	a.sessions = make([]Session, 0, len(consumers))

	for _, cfg := range consumers {
		session, err := a.driver.Open(ctx, cfg)
		if err != nil {
			cancel()
			for _, s := range a.sessions {
				_ = s.Close()
			}
			a.sessions = nil
			return err
		}
		a.sessions = append(a.sessions, session)
		go a.consumeLoop(ctx, cfg, session)
	}

	a.started = true
	return nil
}

// Stop 停止 MQ adapter，并关闭所有已打开的 session。
func (a *Adapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	for _, s := range a.sessions {
		_ = s.Close()
	}
	a.sessions = nil
	a.started = false
	return nil
}

// consumeLoop 持续从一个 session 轮询消息，并在拿到消息后交给 dispatchMessage。
// consumeLoop 持续从单个 session 拉取消息。
// 它故意把“取消息”和“处理消息”分开：前者由 driver 决定，后者统一走 dispatchMessage。
func (a *Adapter) consumeLoop(ctx stdctx.Context, cfg ConsumerConfig, session Session) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, err := session.Poll(ctx)
		if err != nil {
			if errors.Is(err, stdctx.Canceled) || ctx.Err() != nil {
				return
			}
			continue
		}
		if msg == nil {
			continue
		}
		a.dispatchMessage(ctx, cfg, msg)
	}
}

// dispatchMessage 把一条 MQ 消息转换为 runtime.HandlerContext，并交给 runtime 执行。
// dispatchMessage 把 transport-level MQ 消息投影成统一的 HandlerContext。
// 这样业务 handler 就能像其他协议一样，复用 runtime.Execute + binding/mq 整套参数解析。
func (a *Adapter) dispatchMessage(ctx stdctx.Context, cfg ConsumerConfig, msg *Message) {
	if msg == nil || a.host == nil {
		return
	}
	ack := onceAck(msg.Ack)
	nack := onceNack(msg.Nack)
	rctx := runtime.AcquireHandlerContext(mqprotocol.Protocol)
	defer runtime.ReleaseHandlerContext(rctx)

	if ctx == nil {
		ctx = stdctx.Background()
	}
	rctx.WithContext(ctx)
	rctx.Request.Method = msg.Topic
	rctx.Request.Path = cfg.Consumer
	rctx.Request.Body = msg.Raw
	rctx.Request.BodyBytes = append([]byte{}, msg.Body...)
	rctx.Request.Raw = msg.Raw

	headers := normalizeHeaders(msg.Headers)
	for k, values := range headers {
		if len(values) > 0 {
			rctx.Request.Headers[k] = values[0]
		}
		rctx.Request.HeadersMulti[k] = append([]string{}, values...)
	}

	rctx.Set(mqbinding.MetadataKeyHeaders, headers)
	rctx.Set(mqbinding.MetadataKeyTopic, msg.Topic)
	rctx.Set(mqbinding.MetadataKeyConsumer, cfg.Consumer)
	rctx.Set(mqbinding.MetadataKeyKey, msg.Key)
	rctx.Set(mqbinding.MetadataKeyRetryCount, msg.RetryCount)
	if ack != nil {
		rctx.Set(mqbinding.MetadataKeyAck, ack)
	}
	if nack != nil {
		rctx.Set(mqbinding.MetadataKeyNack, nack)
	}

	routeKey := runtime.BuildRouteKey(mqprotocol.Protocol, msg.Topic, cfg.Consumer)
	rctx.RouteKey = routeKey

	_, err := a.host.Runtime().Execute(rctx)
	if err != nil {
		if nack != nil {
			// 执行失败时优先走 nack；ack/nack 都经过 once 包装，避免重复确认。
			_ = nack(err)
		}
		return
	}
	if ack != nil {
		_ = ack()
	}
}

// collectConsumerConfigs 负责把 registry 里分散的 consumer/subscription 声明收拢成 transport 配置。
// 其中：
// - `consumers` 提供 consumer/group
// - `subscriptions` 提供该 consumer 实际订阅哪些 topic
func collectConsumerConfigs(reg *registry.Registry) []ConsumerConfig {
	if reg == nil {
		return nil
	}
	items := reg.ListDecl(AdapterID, "consumers")
	subItems := reg.ListDecl(AdapterID, "subscriptions")

	topicsByConsumer := make(map[string][]string)
	for _, item := range subItems {
		def, ok := item.(*SubscriptionDefinition)
		if !ok || def == nil {
			continue
		}
		topicsByConsumer[def.Consumer] = appendUnique(topicsByConsumer[def.Consumer], def.Topic)
	}

	out := make([]ConsumerConfig, 0, len(items))
	for _, item := range items {
		def, ok := item.(*ConsumerDefinition)
		if !ok || def == nil {
			continue
		}
		out = append(out, ConsumerConfig{
			Consumer: def.Consumer,
			Group:    def.Group,
			Topics:   append([]string{}, topicsByConsumer[def.Consumer]...),
		})
	}
	return out
}

func appendUnique(items []string, v string) []string {
	for _, item := range items {
		if item == v {
			return items
		}
	}
	return append(items, v)
}

// normalizeHeaders 统一把 header key 转成小写，贴合大多数 MQ/gRPC metadata 的大小写不敏感习惯。
func normalizeHeaders(src map[string][]string) map[string][]string {
	out := make(map[string][]string, len(src))
	for k, values := range src {
		key := strings.ToLower(k)
		out[key] = append([]string{}, values...)
	}
	return out
}

// onceAck / onceNack 保证 transport 确认动作最多执行一次。
// 这样即使业务 handler、error 路径或上层重试逻辑重复触发，也不会重复 ack/nack 同一条消息。
func onceAck(fn func() error) func() error {
	if fn == nil {
		return nil
	}
	var once sync.Once
	var err error
	return func() error {
		once.Do(func() {
			err = fn()
		})
		return err
	}
}

func onceNack(fn func(error) error) func(error) error {
	if fn == nil {
		return nil
	}
	var once sync.Once
	var retErr error
	return func(err error) error {
		once.Do(func() {
			retErr = fn(err)
		})
		return retErr
	}
}

var _ adapter.Adapter = (*Adapter)(nil)
