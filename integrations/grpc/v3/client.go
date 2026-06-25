// client.go 实现 grpc integration 的客户端封装与基础连接/调用能力。
package grpc

import (
	stdctx "context"
	"fmt"
	"time"

	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config 描述 gRPC 客户端的初始化配置。
type Config struct {
	Name        string
	Target      string
	DialTimeout time.Duration
	Insecure    bool
	DialOptions []gogrpc.DialOption
	CallOptions []gogrpc.CallOption
}

// Factory 定义 gRPC Client 的工厂接口。
type Factory interface {
	Default() *Client
	New(cfg Config) (*Client, error)
	DialConn(cfg Config) (*gogrpc.ClientConn, error)
	NewStream(ctx stdctx.Context, desc *gogrpc.StreamDesc, method string, opts ...gogrpc.CallOption) (gogrpc.ClientStream, error)
}

// Client 封装一个 gRPC 连接及其默认调用配置。
type Client struct {
	cfg  Config
	conn *gogrpc.ClientConn
}

// New 基于配置创建一个 gRPC Client。
func New(cfg Config) (*Client, error) {
	conn, err := Dial(cfg)
	if err != nil {
		return nil, err
	}
	cfg = normalizeConfig(cfg)
	return &Client{cfg: cfg, conn: conn}, nil
}

// Dial 按配置建立一个新的 gRPC 连接。
func Dial(cfg Config) (*gogrpc.ClientConn, error) {
	cfg = normalizeConfig(cfg)
	if cfg.Target == "" {
		return nil, fmt.Errorf("grpc client requires target")
	}
	opts := append([]gogrpc.DialOption{}, cfg.DialOptions...)
	if cfg.Insecure {
		opts = append(opts, gogrpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	ctx := stdctx.Background()
	cancel := func() {}
	if cfg.DialTimeout > 0 {
		ctx, cancel = stdctx.WithTimeout(ctx, cfg.DialTimeout)
	}
	defer cancel()
	//lint:ignore SA1019 Keep DialContext here to preserve the existing context-aware dial timeout behavior.
	return gogrpc.DialContext(ctx, cfg.Target, opts...)
}

func normalizeConfig(cfg Config) Config {
	if cfg.Name == "" {
		cfg.Name = "default"
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 10 * time.Second
	}
	if !cfg.Insecure && len(cfg.DialOptions) == 0 {
		cfg.Insecure = true
	}
	cfg.DialOptions = append([]gogrpc.DialOption{}, cfg.DialOptions...)
	cfg.CallOptions = append([]gogrpc.CallOption{}, cfg.CallOptions...)
	return cfg
}

// Default 返回当前 Client 本身，用于满足 Factory 接口。
func (c *Client) Default() *Client { return c }

// New 基于 cfg 创建一个新的 Client，用于满足 Factory 接口。
func (c *Client) New(cfg Config) (*Client, error) { return New(cfg) }

// DialConn 按配置建立一个新的裸 gRPC 连接。
func (c *Client) DialConn(cfg Config) (*gogrpc.ClientConn, error) { return Dial(cfg) }

// NewStream 使用当前连接创建一个 gRPC client stream，并自动拼接默认 call options。
func (c *Client) NewStream(ctx stdctx.Context, desc *gogrpc.StreamDesc, method string, opts ...gogrpc.CallOption) (gogrpc.ClientStream, error) {
	// 和 Invoke 一样，这里会先拼上 integration 级默认 CallOptions，
	// 这样超时、metadata、重试等默认策略在 streaming 场景下也能复用。
	callOpts := append(append([]gogrpc.CallOption{}, c.cfg.CallOptions...), opts...)
	return c.conn.NewStream(ctx, desc, method, callOpts...)
}

// Config 返回当前 Client 的配置快照。
func (c *Client) Config() Config { return cloneConfig(c.cfg) }

// Conn 返回底层 gRPC 连接。
func (c *Client) Conn() *gogrpc.ClientConn { return c.conn }

// Close 关闭底层 gRPC 连接。
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// OnModuleDestroy 在模块销毁阶段关闭连接。
func (c *Client) OnModuleDestroy() error { return c.Close() }

// Invoke 使用当前连接直接发起一次 gRPC 调用。
func (c *Client) Invoke(ctx stdctx.Context, method string, args, reply any, opts ...gogrpc.CallOption) error {
	callOpts := append(append([]gogrpc.CallOption{}, c.cfg.CallOptions...), opts...)
	return c.conn.Invoke(ctx, method, args, reply, callOpts...)
}

func cloneConfig(cfg Config) Config {
	out := cfg
	out.DialOptions = append([]gogrpc.DialOption{}, cfg.DialOptions...)
	out.CallOptions = append([]gogrpc.CallOption{}, cfg.CallOptions...)
	return out
}
