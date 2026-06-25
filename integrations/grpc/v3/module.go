// module.go 负责把 grpc integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package grpc

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"

	gogrpc "google.golang.org/grpc"
)

// Module 是 gRPC 客户端集成对应的模块封装。
type Module struct {
	*module.BaseModule
	client       *Client
	config       Config
	reg          *registry.Registry
	compatSource string
}

// ForRoot 创建 gRPC 客户端集成模块，并把默认 client、连接、配置与工厂注册到容器中。
func ForRoot(cfg Config) (module.Module, error) {
	client, err := New(cfg)
	if err != nil {
		return nil, err
	}
	cfg = client.Config()

	clientToken := reflect.TypeFor[*Client]()
	namedClientToken := ClientToken(cfg.Name)
	connToken := reflect.TypeFor[*gogrpc.ClientConn]()
	namedConnToken := ConnToken(cfg.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pClient, err := di.ProviderFromInstanceWithToken(clientToken, client, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedClient, err := di.ProviderFromInstanceWithToken(namedClientToken, client, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConn, err := di.ProviderFromInstanceWithToken(connToken, client.Conn(), di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedConn, err := di.ProviderFromInstanceWithToken(namedConnToken, client.Conn(), di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigPtr, err := di.ProviderFromInstanceWithToken(configPtrToken, &cfg, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigValue, err := di.ProviderFromInstanceWithToken(configValueToken, cfg, di.Singleton)
	if err != nil {
		return nil, err
	}
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(client), di.Singleton)
	if err != nil {
		return nil, err
	}

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pClient, pNamedClient, pConn, pNamedConn, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{clientToken, namedClientToken, connToken, namedConnToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{BaseModule: base, client: client, config: cfg}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
	client, err := New(cfg)
	if err != nil {
		return nil, err
	}
	cfg = client.Config()

	clientToken := reflect.TypeFor[*Client]()
	namedClientToken := ClientToken(cfg.Name)
	connToken := reflect.TypeFor[*gogrpc.ClientConn]()
	namedConnToken := ConnToken(cfg.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pClient, err := di.ProviderFromInstanceWithToken(clientToken, client, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedClient, err := di.ProviderFromInstanceWithToken(namedClientToken, client, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConn, err := di.ProviderFromInstanceWithToken(connToken, client.Conn(), di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedConn, err := di.ProviderFromInstanceWithToken(namedConnToken, client.Conn(), di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigPtr, err := di.ProviderFromInstanceWithToken(configPtrToken, &cfg, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigValue, err := di.ProviderFromInstanceWithToken(configValueToken, cfg, di.Singleton)
	if err != nil {
		return nil, err
	}
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(client), di.Singleton)
	if err != nil {
		return nil, err
	}

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pClient, pNamedClient, pConn, pNamedConn, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{clientToken, namedClientToken, connToken, namedConnToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{compatSource: "integrations/grpc.ForRootCompat", BaseModule: base, client: client, config: cfg}, nil
}

// Init 初始化 gRPC 集成模块，并把 gRPC 相关 binding 注册到 registry。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("grpc.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use grpc.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntryFor[*Client]("GRPCClient"),
		coreintegration.NewBindingEntry("GRPCClientConn", reflect.TypeFor[*gogrpc.ClientConn]()),
		coreintegration.NewBindingEntry("GRPCClientFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("GRPCClientConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("GRPCClientConfigPtr", reflect.TypeFor[*Config]()),
	)
	RegisterBindings(reg)
	return nil
}

// Destroy 销毁 gRPC 模块。
func (m *Module) Destroy() error {
	return m.BaseModule.Destroy()
}

// Client 返回当前模块持有的默认 gRPC client。
func (m *Module) Client() *Client { return m.client }

// Config 返回当前模块使用的 gRPC client 配置快照。
func (m *Module) Config() Config { return cloneConfig(m.config) }

// SetRegistry 注入 registry，供 Init 阶段注册绑定声明。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
