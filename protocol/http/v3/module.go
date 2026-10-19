// module.go 把 HTTP 适配器装配成一个模块。
//
// 这是 HTTP 协议的唯一入口：
//
//	module.New(module.Metadata{
//		Imports: []module.Module{
//			http.Module(http.Config{Addr: ":8080", Validator: true}),
//		},
//		Controllers: []any{&UserController{}},
//	})
package http

import (
	"fmt"
	"net/http"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
)

// Config 描述 HTTP 协议的装配参数。
type Config struct {
	// Addr 是监听地址，例如 ":8080"。
	Addr string
	// BasePath 给所有路由加统一前缀。
	BasePath string
	// Validator 为 true 时启用 go-playground/validator v10。
	Validator bool
	// ValidatorImpl 可注入自定义校验器（优先级高于 Validator）。
	ValidatorImpl httpbinding.Validator
	// Renderer 注入模板渲染器，供 Render 使用。
	Renderer httpbinding.Renderer
	// MaxBodyBytes 限制请求体大小，0 表示默认 10MB。
	MaxBodyBytes int64
	// Middlewares 是 adapter 层中间件，围绕 runtime 执行形成 gin 风格链路。
	Middlewares []func(*httpbinding.HttpContext) error
	// CORS 非空时启用内建 CORS。
	CORS *CorsConfig
	// Helmet 非空时启用内建安全响应头。
	Helmet *HelmetConfig
	// NotFound 是路由未命中时的兜底 handler。
	NotFound http.Handler
}

// Module 创建 HTTP 协议模块。
//
// 返回的模块负责三件事：
//   - 向应用暴露 adapter
//   - 在声明阶段遍历模块树，收集实现了 Controller 的控制器路由
//   - 启动/停止 HTTP 服务
func Module(cfg Config) module.Module {
	a := newAdapter(cfg)
	metadata := module.Metadata{}
	token := reflect.TypeFor[*Adapter]()
	if provider, err := di.ProviderFromInstanceWithToken(token, a, di.Singleton); err == nil {
		metadata.Providers = []*di.Provider{provider}
		metadata.Exports = []any{token}
	}
	return &httpModule{BaseModule: module.New(metadata), adapter: a}
}

type httpModule struct {
	*module.BaseModule
	adapter *Adapter
}

// Adapter 返回模块持有的适配器，供应用在装配阶段收集。
func (m *httpModule) Adapter() adapter.Adapter { return m.adapter }

// Delete 声明阶段：遍历模块树，把控制器路由收集进声明仓库。
//
// 该阶段保证“路由由所属模块提供”：控制器来自某个模块的 Controllers 槽位，
// 其路由声明在该模块的上下文中完成。
func (m *httpModule) Declare(root *module.ModuleRef) error {
	if root == nil || m.adapter == nil || m.adapter.host == nil {
		return nil
	}
	reg := m.adapter.host.Registry()
	if reg == nil {
		return fmt.Errorf("http declare requires an application registry")
	}
	// 声明阶段可重复执行（热加载会重新收集），先清空旧路由。
	reg.ClearDecl(AdapterID, "routes")

	var errs []error
	err := module.WalkModules(root.GetRootModule(), func(mod module.Module) error {
		metadata := mod.Metadata()
		if metadata == nil {
			return nil
		}
		for _, controller := range metadata.Controllers {
			instance, err := module.ResolveController(mod, controller)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			declarer, ok := instance.(Controller)
			if !ok {
				continue
			}
			routes := newRoutes(reg, instance)
			declarer.Declare(routes)
			defs, err := routes.Defs()
			if err != nil {
				errs = append(errs, err)
				continue
			}
			items := make([]any, 0, len(defs))
			for _, def := range defs {
				items = append(items, def)
			}
			reg.RegisterDecl(AdapterID, "routes", items...)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("http route declaration failed: %v", errs)
	}
	return nil
}

// Mountable 由需要挂载原生 net/http handler 的模块实现。
//
// 它让 swagger / terminus 这类模块在不依赖 Application 的前提下完成挂载。
type Mountable interface {
	MountHTTP(adapter.HTTPMountRequest) error
}
