// module.go 把 GraphQL adapter 装配成一个模块。
//
// 这是该协议的唯一入口：把 graphql.Module(cfg) 放进根模块的 Imports 即可。
package graphql

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	gqlbinding "github.com/sao-lang/lania-g/protocol/graphql/v3/binding"
)

// Config 描述 GraphQL 协议的装配参数。
type Config struct {
	// Addr 是监听地址，例如 ":8082"。
	Addr string
	// Path 是 GraphQL 端点路径，默认 /graphql。
	Path string
	// Playground 为 true 时挂载 GraphQL Playground。
	Playground bool
	// PlaygroundPath 是 Playground 路径，默认 /playground。
	PlaygroundPath string
	// MaxBodyBytes 限制请求体大小，0 表示默认 4MB。
	MaxBodyBytes int64
	// Validator 为 true 时启用 go-playground/validator v10。
	Validator bool
	// ValidatorImpl 可注入自定义校验器（优先级高于 Validator）。
	ValidatorImpl gqlbinding.Validator
	// ErrorFormatter 自定义错误输出形态。
	ErrorFormatter func(error) any
	// ContextFactory 自定义每次请求的 GraphQLContext 构造方式。
	ContextFactory func(*http.Request) *gqlbinding.GraphQLContext
}

// Module 创建 GraphQL 协议模块。
func Module(cfg Config) module.Module {
	a := New(cfg.Addr)
	if strings.TrimSpace(cfg.Path) != "" {
		a = a.WithPath(cfg.Path)
	}
	if cfg.Playground {
		a = a.MountPlayground(cfg.PlaygroundPath)
	}
	if cfg.MaxBodyBytes > 0 {
		a = a.WithMaxBodyBytes(cfg.MaxBodyBytes)
	}
	if cfg.ValidatorImpl != nil {
		a = a.WithValidator(cfg.ValidatorImpl)
	} else if cfg.Validator {
		a = a.WithValidatorV10()
	}
	if cfg.ErrorFormatter != nil {
		a = a.WithErrorFormatter(cfg.ErrorFormatter)
	}
	if cfg.ContextFactory != nil {
		a = a.WithContextFactory(cfg.ContextFactory)
	}
	m := &protocolModule{BaseModule: module.New(module.Metadata{})}
	m.adapter = a
	return m
}

type protocolModule struct {
	*module.BaseModule
	adapter *Adapter
}

// Adapter 返回模块持有的适配器，供应用在装配阶段收集。
func (m *protocolModule) Adapter() adapter.Adapter { return m.adapter }

var _ adapter.ModuleAdapter = (*protocolModule)(nil)

// Declare 收集实现了 graphql.Controller 的 resolver 声明。
func (m *protocolModule) Declare(root *module.ModuleRef) error {
	if root == nil || m.adapter == nil || m.adapter.reg == nil {
		return nil
	}
	reg := m.adapter.reg
	reg.ClearDecl(AdapterID, "resolvers")
	var errs []error
	err := module.WalkModules(root.GetRootModule(), func(mod module.Module) error {
		metadata := mod.Metadata()
		if metadata == nil {
			return nil
		}
		for _, resolver := range metadata.Resolvers {
			instance, err := module.ResolveController(mod, resolver)
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
			if err := routes.Err(); err != nil {
				errs = append(errs, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("graphql declaration failed: %v", errs)
	}
	return nil
}
