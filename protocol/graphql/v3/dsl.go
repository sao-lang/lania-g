// dsl.go 提供 GraphQL adapter 的声明式注册 DSL。
// 这层代码的职责是把“用户写出来的链式声明”收集成 registry declaration，
// 真正的编译、schema 构建和执行逻辑由 plugin / schema_* / execution 等文件负责。
package graphql

import "github.com/sao-lang/lania-g/kernel/v3/aop"

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	coreadapter "github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// ArgBuilder 描述一个 GraphQL 字段参数的声明信息。
type ArgBuilder struct {
	name         string
	description  string
	required     bool
	defaultValue any
}

// ArgOption 用于以函数式方式修改 ArgBuilder。
type ArgOption func(*ArgBuilder)

// WithDescription 为 GraphQL 参数补充描述信息。
func WithDescription(desc string) ArgOption {
	return func(ab *ArgBuilder) {
		ab.description = desc
	}
}

// WithRequired 标记该参数是否必填。
func WithRequired(required bool) ArgOption {
	return func(ab *ArgBuilder) {
		ab.required = required
	}
}

// WithDefault 为 GraphQL 参数设置默认值。
func WithDefault(value any) ArgOption {
	return func(ab *ArgBuilder) {
		ab.defaultValue = value
	}
}

// Controller 由需要声明 GraphQL resolver 的控制器实现。
type Controller interface {
	Declare(*Routes)
}

// Routes 收集一个控制器声明的全部 GraphQL resolver 与 schema 配置。
//
// 它是 GraphQL 唯一的声明入口：控制器在所属模块内完成注册，无需手动 Build。
type Routes struct {
	reg      *registry.Registry
	resolver any
	current  *ResolverBuilder
	err      error
}

// NewRoutes 创建一个绑定到 reg 的声明收集器。
//
// 正常装配由 adapter 在 Declare 阶段构造；测试或自定义装载器可直接使用。
func NewRoutes(reg *registry.Registry, resolver any) *Routes {
	return &Routes{reg: reg, resolver: resolver}
}

// Resolver 开始声明一个 GraphQL resolver。
func (r *Routes) Resolver(name string) *ResolverBuilder {
	if err := r.sealCurrent(); err != nil && r.err == nil {
		r.err = err
	}
	b := newResolverBuilder(name, r.resolver, r)
	r.current = b
	return b
}

// UseSchema 为当前模块声明 GraphQL schema 配置。
func (r *Routes) UseSchema(schema *Schema) *Routes {
	if schema != nil && r.reg != nil {
		r.reg.RegisterDecl(AdapterID, "config", &ConfigDecl{Schema: schema})
	}
	return r
}

// DisableIntrospection 关闭 introspection。
func (r *Routes) DisableIntrospection() *Routes {
	if r.reg != nil {
		r.reg.RegisterDecl(AdapterID, "config", &ConfigDecl{DisableIntrospection: true})
	}
	return r
}

// UseExtensions 注册 GraphQL 扩展能力。
func (r *Routes) UseExtensions(extensions ...Extension) *Routes {
	if len(extensions) > 0 && r.reg != nil {
		r.reg.RegisterDecl(AdapterID, "config", &ConfigDecl{Extensions: extensions})
	}
	return r
}

// UseComplexityLimit 设置查询复杂度上限。
func (r *Routes) UseComplexityLimit(limit int) *Routes {
	if limit > 0 && r.reg != nil {
		r.reg.RegisterDecl(AdapterID, "config", &ConfigDecl{ComplexityLimit: limit})
	}
	return r
}

// Err 结束声明并返回累计错误。
func (r *Routes) Err() error {
	if r == nil {
		return nil
	}
	if err := r.sealCurrent(); err != nil && r.err == nil {
		r.err = err
	}
	return r.err
}

func (r *Routes) sealCurrent() error {
	if r == nil || r.current == nil {
		return nil
	}
	b := r.current
	r.current = nil
	return b.seal()
}

// ResolverBuilder 用于声明一个 resolver 及其字段集合。
type ResolverBuilder struct {
	name           string
	resolver       any
	fields         []*FieldBuilder
	guards         []aop.Guard
	interceptors   []aop.Interceptor
	middlewares    []aop.Middleware
	pipes          []aop.Pipe
	paramPipes     map[int][]aop.Pipe
	filters        []aop.ExceptionFilter
	routes         *Routes
	sealed         bool
	err            error
	mu             sync.RWMutex
}

// FieldBuilder 用于声明 resolver 上的一个具体字段。
type FieldBuilder struct {
	resolverBuilder *ResolverBuilder
	fieldType       FieldType
	fieldName       string
	handler         any
	handlerName     string
	returns         string
	description     string
	deprecation     string
	args            []*ArgBuilder
	guards          []aop.Guard
	interceptors    []aop.Interceptor
	middlewares     []aop.Middleware
	pipes           []aop.Pipe
	paramPipes      map[int][]aop.Pipe
	filters         []aop.ExceptionFilter
	cacheControl    string
	permissions     []string
	timeout         int64
	complexity      int
	rateLimit       *RateLimitConfig
}

// newResolverBuilder 创建一个可继续链式追加字段声明的 builder。
// 如果没有显式传入 registry，则回退到全局 registry，以兼容旧 DSL 用法。
func newResolverBuilder(name string, resolver any, routes *Routes) *ResolverBuilder {
	return &ResolverBuilder{
		name:       name,
		resolver:   resolver,
		fields:     make([]*FieldBuilder, 0),
		paramPipes: make(map[int][]aop.Pipe),
		routes:     routes,
	}
}

// UseGuards 为 resolver 级别追加 guards。
func (rb *ResolverBuilder) UseGuards(items ...aop.Guard) *ResolverBuilder {
	rb.guards = append(rb.guards, items...)
	return rb
}

// UseInterceptors 为 resolver 级别追加 interceptors。
func (rb *ResolverBuilder) UseInterceptors(items ...aop.Interceptor) *ResolverBuilder {
	rb.interceptors = append(rb.interceptors, items...)
	return rb
}

// UseMiddlewares 为 resolver 级别追加 middlewares。
func (rb *ResolverBuilder) UseMiddlewares(items ...aop.Middleware) *ResolverBuilder {
	rb.middlewares = append(rb.middlewares, items...)
	return rb
}

// UsePipes 为 resolver 级别追加 pipes。
func (rb *ResolverBuilder) UsePipes(items ...aop.Pipe) *ResolverBuilder {
	rb.pipes = append(rb.pipes, items...)
	return rb
}

// UseParamPipes 为指定参数位置追加 pipes。
func (rb *ResolverBuilder) UseParamPipes(paramIndex int, items ...aop.Pipe) *ResolverBuilder {
	if rb.paramPipes[paramIndex] == nil {
		rb.paramPipes[paramIndex] = make([]aop.Pipe, 0)
	}
	rb.paramPipes[paramIndex] = append(rb.paramPipes[paramIndex], items...)
	return rb
}

// UseFilters 为 resolver 级别追加 filters。
func (rb *ResolverBuilder) UseFilters(items ...aop.ExceptionFilter) *ResolverBuilder {
	rb.filters = append(rb.filters, items...)
	return rb
}

// Query 在当前 resolver 下声明一个 Query 字段。
func (rb *ResolverBuilder) Query(fieldName string, handler any) *FieldBuilder {
	return rb.addField(FieldTypeQuery, fieldName, handler)
}

// Mutation 在当前 resolver 下声明一个 Mutation 字段。
func (rb *ResolverBuilder) Mutation(fieldName string, handler any) *FieldBuilder {
	return rb.addField(FieldTypeMutation, fieldName, handler)
}

// Subscription 在当前 resolver 下声明一个 Subscription 字段（当前版本暂不支持编译）。
func (rb *ResolverBuilder) Subscription(fieldName string, handler any) *FieldBuilder {
	return rb.addField(FieldTypeSubscription, fieldName, handler)
}

// Object 在当前 resolver 下声明一个 Object 字段（用于嵌套选择集分发）。
func (rb *ResolverBuilder) Object(fieldName string, handler any) *FieldBuilder {
	return rb.addField(FieldTypeObject, fieldName, handler)
}

func (rb *ResolverBuilder) addField(fieldType FieldType, fieldName string, handler any) *FieldBuilder {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.sealed {
		// sealed 之后不再允许继续追加字段，保持 builder 输出稳定。
		return nil
	}

	fb := &FieldBuilder{
		resolverBuilder: rb,
		fieldType:       fieldType,
		fieldName:       fieldName,
		handler:         handler,
		args:            make([]*ArgBuilder, 0),
		guards:          make([]aop.Guard, 0),
		interceptors:    make([]aop.Interceptor, 0),
		middlewares:     make([]aop.Middleware, 0),
		pipes:           make([]aop.Pipe, 0),
		paramPipes:      make(map[int][]aop.Pipe),
		filters:         make([]aop.ExceptionFilter, 0),
	}

	val := reflect.ValueOf(handler)
	if val.Kind() == reflect.Func {
		// DSL 收到的是绑定方法值，这里先把 methodName 推导出来，
		// 后续编译阶段再按 receiver token + methodName 构造 runtime.Handler。
		fb.handlerName = coreadapter.FindMethodName(rb.resolver, handler)
	}

	rb.fields = append(rb.fields, fb)
	return fb
}

// seal 完成 resolver 声明并写入 registry。
func (rb *ResolverBuilder) seal() error {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.sealed {
		return rb.err
	}
	rb.sealed = true
	if err := rb.validateLocked(); err != nil {
		rb.err = err
		return err
	}
	fields := make([]*FieldDefinition, 0, len(rb.fields))
	for _, fb := range rb.fields {
		fields = append(fields, fb.build())
	}
	def := &ResolverDefinition{
		Name:         rb.name,
		Resolver:     rb.resolver,
		Fields:       fields,
		Guards:       append([]aop.Guard{}, rb.guards...),
		Interceptors: append([]aop.Interceptor{}, rb.interceptors...),
		Middlewares:  append([]aop.Middleware{}, rb.middlewares...),
		Pipes:        append([]aop.Pipe{}, rb.pipes...),
		ParamPipes:   coreadapter.MergeParamPipes(rb.paramPipes, nil),
		Filters:      append([]aop.ExceptionFilter{}, rb.filters...),
	}
	if rb.routes != nil && rb.routes.reg != nil {
		rb.routes.reg.RegisterDecl(AdapterID, "resolvers", def)
	}
	return nil
}

// Err 返回 DSL 构建过程中记录下来的错误。
func (rb *ResolverBuilder) Err() error { return rb.err }

// Description 设置字段的文档描述。
func (fb *FieldBuilder) Description(desc string) *FieldBuilder {
	fb.description = desc
	return fb
}

// Return 设置字段返回的 GraphQL 类型名。
func (fb *FieldBuilder) Return(typeName string) *FieldBuilder {
	fb.returns = strings.TrimSpace(typeName)
	return fb
}

// Returns 与 Return 等价，保留链式风格。
func (fb *FieldBuilder) Returns(typeName string) *FieldBuilder {
	return fb.Return(typeName)
}

// ObjectType 与 Return 等价，语义上强调“返回对象类型”。
func (fb *FieldBuilder) ObjectType(typeName string) *FieldBuilder {
	return fb.Return(typeName)
}

// Deprecation 设置字段废弃原因。
func (fb *FieldBuilder) Deprecation(reason string) *FieldBuilder {
	fb.deprecation = reason
	return fb
}

// Arg 为字段追加一个参数声明。
func (fb *FieldBuilder) Arg(name string, options ...ArgOption) *FieldBuilder {
	ab := &ArgBuilder{name: name}
	for _, opt := range options {
		opt(ab)
	}
	fb.args = append(fb.args, ab)
	return fb
}

// Args 批量追加字段参数声明。
func (fb *FieldBuilder) Args(args ...*ArgBuilder) *FieldBuilder {
	fb.args = append(fb.args, args...)
	return fb
}

// UseGuards 为字段级别追加 guards。
func (fb *FieldBuilder) UseGuards(items ...aop.Guard) *FieldBuilder {
	fb.guards = append(fb.guards, items...)
	return fb
}

// UseInterceptors 为字段级别追加 interceptors。
func (fb *FieldBuilder) UseInterceptors(items ...aop.Interceptor) *FieldBuilder {
	fb.interceptors = append(fb.interceptors, items...)
	return fb
}

// UseMiddlewares 为字段级别追加 middlewares。
func (fb *FieldBuilder) UseMiddlewares(items ...aop.Middleware) *FieldBuilder {
	fb.middlewares = append(fb.middlewares, items...)
	return fb
}

// UsePipes 为字段级别追加 pipes。
func (fb *FieldBuilder) UsePipes(items ...aop.Pipe) *FieldBuilder {
	fb.pipes = append(fb.pipes, items...)
	return fb
}

// UseParamPipes 为指定参数位置追加 pipes。
func (fb *FieldBuilder) UseParamPipes(paramIndex int, items ...aop.Pipe) *FieldBuilder {
	if fb.paramPipes[paramIndex] == nil {
		fb.paramPipes[paramIndex] = make([]aop.Pipe, 0)
	}
	fb.paramPipes[paramIndex] = append(fb.paramPipes[paramIndex], items...)
	return fb
}

// UseFilters 为字段级别追加 filters。
func (fb *FieldBuilder) UseFilters(items ...aop.ExceptionFilter) *FieldBuilder {
	fb.filters = append(fb.filters, items...)
	return fb
}

// CacheControl 设置字段的 cache-control 文档信息。
func (fb *FieldBuilder) CacheControl(control string) *FieldBuilder {
	fb.cacheControl = control
	return fb
}

// RequirePermission 为字段追加权限标记（具体执行依赖业务侧 guard/interceptor）。
func (fb *FieldBuilder) RequirePermission(permission string) *FieldBuilder {
	fb.permissions = append(fb.permissions, permission)
	return fb
}

// Timeout 设置字段执行超时（单位毫秒）。
func (fb *FieldBuilder) Timeout(milliseconds int64) *FieldBuilder {
	fb.timeout = milliseconds
	return fb
}

// Complexity 设置字段复杂度。
func (fb *FieldBuilder) Complexity(complexity int) *FieldBuilder {
	fb.complexity = complexity
	return fb
}

// RateLimit 设置字段限流配置。
func (fb *FieldBuilder) RateLimit(limit int, period int64) *FieldBuilder {
	fb.rateLimit = &RateLimitConfig{Limit: limit, Period: period}
	return fb
}

// Query 在当前 resolver 下继续声明一个 Query 字段。
func (fb *FieldBuilder) Query(fieldName string, handler any) *FieldBuilder {
	return fb.resolverBuilder.Query(fieldName, handler)
}

// Mutation 在当前 resolver 下继续声明一个 Mutation 字段。
func (fb *FieldBuilder) Mutation(fieldName string, handler any) *FieldBuilder {
	return fb.resolverBuilder.Mutation(fieldName, handler)
}

// Subscription 在当前 resolver 下继续声明一个 Subscription 字段。
func (fb *FieldBuilder) Subscription(fieldName string, handler any) *FieldBuilder {
	return fb.resolverBuilder.Subscription(fieldName, handler)
}

// Object 在当前 resolver 下继续声明一个 Object 字段。
func (fb *FieldBuilder) Object(fieldName string, handler any) *FieldBuilder {
	return fb.resolverBuilder.Object(fieldName, handler)
}

func (fb *FieldBuilder) build() *FieldDefinition {
	args := make([]*ArgDefinition, 0, len(fb.args))
	for _, ab := range fb.args {
		args = append(args, ab.build())
	}
	// 这里不做 runtime 编译，只是把 field 级 DSL 收拢成纯声明对象。
	return &FieldDefinition{
		FieldType:    fb.fieldType,
		FieldName:    fb.fieldName,
		Handler:      fb.handler,
		HandlerName:  fb.handlerName,
		Returns:      fb.returns,
		Description:  fb.description,
		Deprecation:  fb.deprecation,
		Args:         args,
		Guards:       append([]aop.Guard{}, fb.guards...),
		Interceptors: append([]aop.Interceptor{}, fb.interceptors...),
		Middlewares:  append([]aop.Middleware{}, fb.middlewares...),
		Pipes:        append([]aop.Pipe{}, fb.pipes...),
		ParamPipes:   coreadapter.MergeParamPipes(fb.paramPipes, nil),
		Filters:      append([]aop.ExceptionFilter{}, fb.filters...),
		CacheControl: fb.cacheControl,
		Permissions:  append([]string{}, fb.permissions...),
		Timeout:      fb.timeout,
		Complexity:   fb.complexity,
		RateLimit:    fb.rateLimit,
	}
}

func (ab *ArgBuilder) build() *ArgDefinition {
	return &ArgDefinition{
		Name:         ab.name,
		Description:  ab.description,
		Required:     ab.required,
		DefaultValue: ab.defaultValue,
	}
}

func (rb *ResolverBuilder) validateLocked() error {
	if strings.TrimSpace(rb.name) == "" {
		return fmt.Errorf("graphql resolver name is required")
	}
	if rb.resolver == nil {
		return fmt.Errorf("graphql resolver %s is nil", rb.name)
	}
	for _, field := range rb.fields {
		if field == nil {
			return fmt.Errorf("graphql field builder is nil")
		}
		if strings.TrimSpace(field.fieldName) == "" {
			return fmt.Errorf("graphql field name is required")
		}
		if field.handler == nil || strings.TrimSpace(field.handlerName) == "" {
			return fmt.Errorf("invalid graphql field declaration: %s.%s", rb.name, field.fieldName)
		}
		if field.fieldType == FieldTypeSubscription {
			return fmt.Errorf("graphql subscription %s.%s is not supported yet", rb.name, field.fieldName)
		}
	}
	return nil
}
