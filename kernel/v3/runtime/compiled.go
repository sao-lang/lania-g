// compiled.go 定义编译期产物会写入 runtime 的若干共享结构。
//
// 这些类型本身不执行业务逻辑，但它们决定了 compiler 和 runtime 之间如何交换信息。
package runtime

import (
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
)

// ParamPlan 是编译期生成的“参数绑定计划”，由 compiler 产出、runtime 消费。
// runtime.BindingRegistry.Resolve 会根据 handler.Meta.ParamPlans[paramIndex].BindingName 等信息补全 descriptor。
type ParamPlan struct {
	Index       int
	Type        reflect.Type
	BindingName string
}

// CompiledHTTPRoute 表示一条编译期产出的 HTTP 路由元信息。
type CompiledHTTPRoute struct {
	Method   string
	Path     string
	RouteKey string
}

// CompiledHTTPNode 是编译期 HTTP 路由树中的一个节点。
// 它是安装前的中间结构：先在编译期建树，再由 adapter/http 回放到运行时 matcher。
type CompiledHTTPNode struct {
	Static    map[string]*CompiledHTTPNode
	Param     *CompiledHTTPNode
	ParamName string
	RouteKey  string
}

// CompiledHTTPTree 是编译期生成的 HTTP 路由树，用于减少运行时匹配成本。
type CompiledHTTPTree struct {
	Methods map[string]*CompiledHTTPNode
	All     *CompiledHTTPNode
}

// AOPPlan 是编译期生成的 AOP 计划（按顺序展开后的 middleware/guard/interceptor/pipe/filter）。
// pipeline.Run 在 handler.Meta.CompiledAOP != nil 时会直接使用该计划，避免与运行期声明重复合并。
type AOPPlan struct {
	Middlewares  []aop.MiddlewareFunc
	Guards       []aop.GuardFunc
	Interceptors []aop.InterceptorFunc
	Pipes        []aop.PipeFunc
	ParamPipes   map[int][]aop.PipeFunc
	Filters      []aop.ExceptionFilterFunc
}
