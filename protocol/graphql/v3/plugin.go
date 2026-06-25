// plugin.go 负责把 GraphQL DSL/registry 里的声明收敛成运行期可执行的 handler 集合。
// 它不处理 schema 生成本身，而是专注于：
// - 解析 resolver/field 的模块归属
// - 把字段声明编译成 runtime.Handler
// - 生成 GraphQL 路由键并安装到统一编译产物中
package graphql

import (
	"fmt"
	"reflect"
	"strings"

	coreadapter "github.com/sao-lang/lania-g/kernel/v3/adapter"
	gqlbinding "github.com/sao-lang/lania-g/protocol/graphql/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	gqlprotocol "github.com/sao-lang/lania-g/protocol/graphql/v3/protocol"
)

// AdapterID 是 GraphQL 协议插件与 adapter 的统一标识。
const AdapterID = "graphql"

// Plugin 是 GraphQL 协议的编译插件。
type Plugin struct{}

// NewPlugin 创建一个 GraphQL 协议插件实例。
func NewPlugin() compiler.ProtocolPlugin { return &Plugin{} }

// ID 返回插件 ID（同时也是 adapter id）。
func (p *Plugin) ID() string { return AdapterID }

// Protocol 返回当前插件负责的协议标识。
func (p *Plugin) Protocol() runtime.Protocol {
	return gqlprotocol.Protocol
}

// Register 向 registry 注册该协议需要的默认 binding 等依赖。
func (p *Plugin) Register(reg *registry.Registry) {
	gqlbinding.RegisterDefaultsToRegistry(reg)
}

type fieldOwnership struct {
	resolverDef  *ResolverDefinition
	field        *FieldDefinition
	moduleKey    string
	moduleType   reflect.Type
	container    *di.Container
	resolverType reflect.Type
}

type scanResult struct {
	fields []*fieldOwnership
}

// Scan 扫描 registry 中的 GraphQL 声明，并解析每个 resolver/field 的归属模块与容器。
func (p *Plugin) Scan(moduleRef *module.ModuleRef, reg *registry.Registry) (any, error) {
	fields, err := compiler.ScanResolvedRegistryItems(AdapterID, moduleRef, reg,
		"graphql plugin scan requires an explicit registry; pass the application registry instance or registry.Global() explicitly for compatibility",
		compiler.SnapshotOwnerOptions{
			Controllers: true,
			Resolvers:   true,
		},
		"resolvers",
		func(raw any) (*ResolverDefinition, bool) {
			def, ok := raw.(*ResolverDefinition)
			return def, ok && def != nil && def.Resolver != nil
		},
		func(def *ResolverDefinition) any { return def.Resolver },
		func(def *ResolverDefinition) string { return fmt.Sprintf("graphql resolver %s", def.Name) },
		func(def *ResolverDefinition) map[string]any {
			return map[string]any{
				"resolver": def.Name,
			}
		},
		"",
		func(def *ResolverDefinition, own compiler.ModuleOwner, token reflect.Type) []*fieldOwnership {
			// GraphQL 的 owner 是按 resolver 归属解析出来的，但真正要编译的是字段。
			// 因此这里先把“一个 resolver”展开成“多个 field ownership”。
			fields := make([]*fieldOwnership, 0, len(def.Fields))
			for _, field := range def.Fields {
				if field == nil {
					continue
				}
				fields = append(fields, &fieldOwnership{
					resolverDef:  def,
					field:        field,
					moduleKey:    own.ModuleKey,
					moduleType:   own.ModuleType,
					container:    own.Container,
					resolverType: token,
				})
			}
			return fields
		},
	)
	if err != nil {
		return nil, err
	}

	// ScanResolvedRegistryItems 返回的是“每个 resolver 对应一组字段”的二维结果，
	// 编译阶段更适合消费扁平化的字段列表，因此这里再做一次拍平。
	flattened := make([]*fieldOwnership, 0, len(fields))
	for _, owned := range fields {
		flattened = append(flattened, owned...)
	}
	return &scanResult{fields: flattened}, nil
}

// Compile 将扫描结果编译为运行期可执行的路由集合（GraphQL 字段 -> runtime.Handler）。
func (p *Plugin) Compile(scan any, reg *registry.Registry, global registry.GlobalAOPRegistration) (*compiler.CompiledProtocol, error) {
	return compiler.CompileSimpleProtocol(scan, gqlprotocol.Protocol, AdapterID, "invalid scan result for graphql plugin",
		func(scan any) ([]*fieldOwnership, bool) {
			result, ok := scan.(*scanResult)
			if !ok || result == nil {
				return nil, false
			}
			return result.fields, true
		},
		global,
		func(owned *fieldOwnership, global registry.GlobalAOPRegistration) (*compiler.CompiledRoute[*fieldOwnership], error) {
			// compileField 保留 GraphQL 特有逻辑；CompileSimpleProtocol 只负责统一外壳。
			h, routeKey, err := compileField(owned, global)
			if err != nil {
				return nil, err
			}
			return &compiler.CompiledRoute[*fieldOwnership]{
				Item:      owned,
				Handler:   h,
				RouteKey:  routeKey,
				Container: owned.container,
			}, nil
		},
		func(owned *fieldOwnership) string {
			return fmt.Sprintf("duplicate graphql field %s.%s", owned.resolverDef.Name, owned.field.FieldName)
		},
	)
}

// compileField 把单个字段声明编译成 runtime.Handler，并生成 routeKey。
// GraphQL 这里的“路由”并非 HTTP path，而是：
// - Query/Mutation：按字段类型 + 字段名定位
// - Object：按 resolver 名 + 字段名定位
func compileField(item *fieldOwnership, global registry.GlobalAOPRegistration) (*runtime.Handler, string, error) {
	def := item.field
	if def == nil || item.resolverDef == nil {
		return nil, "", fmt.Errorf("nil graphql field definition")
	}
	if def.FieldType == FieldTypeSubscription {
		return nil, "", fmt.Errorf("graphql subscription %s.%s is not supported yet", item.resolverDef.Name, def.FieldName)
	}
	if item.resolverDef.Resolver == nil || def.HandlerName == "" {
		return nil, "", fmt.Errorf("invalid graphql field declaration: %s.%s", item.resolverDef.Name, def.FieldName)
	}

	h, err := runtime.NewHandlerByToken(item.resolverType, def.HandlerName)
	if err != nil {
		return nil, "", err
	}

	// GraphQL 字段既可挂在 resolver 级 AOP，也可挂在字段级 AOP；
	// 这里按“resolver 在前、field 在后”合并，保持“越靠近 handler 越后执行”的直觉。
	plan := compiler.CompileAOPPlan(global, compiler.AOPSources{
		Middlewares:  append(item.resolverDef.Middlewares, def.Middlewares...),
		Guards:       append(item.resolverDef.Guards, def.Guards...),
		Interceptors: append(item.resolverDef.Interceptors, def.Interceptors...),
		Pipes:        append(item.resolverDef.Pipes, def.Pipes...),
		ParamPipes:   coreadapter.MergeParamPipes(item.resolverDef.ParamPipes, def.ParamPipes),
		Filters:      append(item.resolverDef.Filters, def.Filters...),
	})

	h.Meta.CompiledAOP = &plan
	h.Meta.ParamPlans = buildParamPlan(h, def)
	h.Meta.ModuleKey = item.moduleKey

	method := string(def.FieldType)
	// Object 字段在 runtime routeKey 里要用 resolver 名做 method 段，
	// 这样才能和 Query/Mutation 命名空间区分开。
	if def.FieldType == FieldTypeObject {
		method = item.resolverDef.Name
	}
	routeKey := runtime.BuildRouteKey(gqlprotocol.Protocol, method, def.FieldName)
	h.Meta.RouteKey = routeKey
	h.Meta.Protocol = gqlprotocol.Protocol
	return h, routeKey, nil
}

// buildParamPlan 为字段参数生成运行期 ParamPlan。
// 只有 `Arg[T]` / `ArgValue[T]` 这类参数需要写 BindingName，
// 其他参数仍按普通 binding 解析流程处理。
func buildParamPlan(handler *runtime.Handler, def *FieldDefinition) []runtime.ParamPlan {
	out := make([]runtime.ParamPlan, 0, len(handler.Meta.ParamTypes))
	argIndex := 0
	for i, typ := range handler.Meta.ParamTypes {
		plan := runtime.ParamPlan{Index: i, Type: typ}
		if isArgWrapper(typ) && argIndex < len(def.Args) {
			plan.BindingName = def.Args[argIndex].Name
			argIndex++
		}
		out = append(out, plan)
	}
	return out
}

// isArgWrapper 用于识别“当前 handler 参数是否属于 GraphQL 参数 wrapper”。
func isArgWrapper(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct && t.PkgPath() == gqlbinding.PackagePath() && (trimGenericName(t.Name()) == "Arg" || trimGenericName(t.Name()) == "ArgValue")
}

// trimGenericName 抹掉运行时类型名中的泛型尾缀，便于做 wrapper 名称比较。
func trimGenericName(name string) string {
	if head, _, ok := strings.Cut(name, "["); ok {
		return head
	}
	return name
}
