// register.go 负责组装 GraphQL 协议默认启用的 binding 集合。
// 它关注的是“有哪些 binding、如何匹配、如何解析”，
// 而真正的请求期 metadata 由 GraphQL adapter 在执行阶段写入 HandlerContext。
package graphql

import (
	"reflect"

	coreregistry "github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	gqlprotocol "github.com/sao-lang/lania-g/protocol/graphql/v3/protocol"
)

const (
	// 这些 key 是 GraphQL adapter 与 binding 层之间的约定：
	// adapter 在执行字段前写入，binding resolver 再据此把运行时信息注入到参数里。
	// MetadataKeyContext 是 metadata 中保存 GraphQL context 的键。
	MetadataKeyContext       = "graphql.context"
	// MetadataKeyHeaders 是 metadata 中保存请求头映射的键。
	MetadataKeyHeaders       = "graphql.headers"
	// MetadataKeyRoot 是 metadata 中保存 root object 的键。
	MetadataKeyRoot          = "graphql.root"
	// MetadataKeyInfo 是 metadata 中保存 GraphQL resolve info 的键。
	MetadataKeyInfo          = "graphql.info"
	// MetadataKeyVars 是 metadata 中保存变量映射的键。
	MetadataKeyVars          = "graphql.vars"
	// MetadataKeySession 是 metadata 中保存会话对象的键。
	MetadataKeySession       = "graphql.session"
	// MetadataKeyField 是 metadata 中保存当前字段对象的键。
	MetadataKeyField         = "graphql.field"
	// MetadataKeyFieldTyp 是 metadata 中保存字段类型信息的键。
	MetadataKeyFieldTyp      = "graphql.fieldType"
	// MetadataKeySelectionSet 是 metadata 中保存 selection set 的键。
	MetadataKeySelectionSet  = "graphql.selectionSet"
	// MetadataKeyOperationName 是 metadata 中保存操作名的键。
	MetadataKeyOperationName = "graphql.operationName"
	// MetadataKeyRawQuery 是 metadata 中保存原始查询串的键。
	MetadataKeyRawQuery      = "graphql.rawQuery"
	// MetadataKeyExtensions 是 metadata 中保存扩展参数的键。
	MetadataKeyExtensions    = "graphql.extensions"
	// MetadataKeyFieldName 是 metadata 中保存字段名的键。
	MetadataKeyFieldName     = "graphql.fieldName"
	// MetadataKeyValidator 是 metadata 中保存 GraphQL DTO 校验器的键。
	MetadataKeyValidator     = "graphql.validator"
)

// RegisterDefaults 将内置的 GraphQL 参数绑定规则注册到 runtime。
func RegisterDefaults(rt *runtime.Runtime) {
	for _, reg := range DefaultRegistrations() {
		// DefaultRegistrations 只返回描述表；写入 runtime 时统一转换成 resolver。
		rt.RegisterBinding(runtime.NewBindingResolver(reg))
	}
}

// RegisterDefaultsToRegistry 将内置的 GraphQL 参数绑定规则注册到 registry。
// 如果 reg 为空，则回退到全局 registry。
func RegisterDefaultsToRegistry(reg *coreregistry.Registry) {
	if reg == nil {
		RegisterDefaultsCompat()
		return
	}
	registerDefaultsToRegistry(reg)
}

// RegisterDefaultsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterDefaultsCompat() {
	registerDefaultsToRegistry(coreregistry.GlobalWithUsage("binding/graphql.RegisterDefaultsCompat"))
}

func registerDefaultsToRegistry(reg *coreregistry.Registry) {
	reg.RegisterBindings(DefaultResolvers()...)
}

// DefaultRegistrations 返回 GraphQL 协议默认启用的一组 binding registration。
func DefaultRegistrations() []runtime.BindingRegistration {
	// GraphQL binding 只允许在 GraphQL 协议下命中，避免其他协议错误复用。
	allowed := map[runtime.Protocol]bool{gqlprotocol.Protocol: true}
	return []runtime.BindingRegistration{
		// 最常见的上下文/参数/请求头/父节点注入入口。
		registration("GraphQLContext", allowed, matchGraphQLContext, resolveGraphQLContext),
		registration("Arg", allowed, matchGenericWrapper("Arg"), resolveFromArgs),
		registration("ArgValue", allowed, matchGenericWrapper("ArgValue"), resolveFromArgs),
		registration("Header", allowed, matchGenericWrapper("Header"), resolveHeader),
		registration("Parent", allowed, matchGenericWrapper("Parent"), resolveParent),
		registration("Variables", allowed, matchNamedType[Variables]("Variables"), resolveVariables),
		registration("Headers", allowed, matchNamedType[Headers]("Headers"), resolveHeadersMap),
		registration("Extensions", allowed, matchNamedType[Extensions]("Extensions"), resolveExtensions),
		registration("SelectionSet", allowed, matchNamedType[SelectionSet]("SelectionSet"), resolveSelectionSet),
		registration("Root", allowed, matchNamedType[Root]("Root"), resolveRoot),
		registration("Info", allowed, matchNamedType[Info]("Info"), resolveInfo),
		registration("OperationName", allowed, matchNamedType[OperationName]("OperationName"), resolveOperationName),
		registration("FieldName", allowed, matchNamedType[FieldName]("FieldName"), resolveFieldName),
		registration("RawQuery", allowed, matchNamedType[RawQuery]("RawQuery"), resolveRawQuery),
		registration("IP", allowed, matchNamedType[IP]("IP"), resolveIP),
		registration("Host", allowed, matchNamedType[Host]("Host"), resolveHost),
		registration("Method", allowed, matchNamedType[Method]("Method"), resolveMethod),
		registration("URL", allowed, matchNamedType[URL]("URL"), resolveURL),
		registration("Path", allowed, matchNamedType[Path]("Path"), resolvePath),
		registration("Session", allowed, matchNamedType[Session]("Session"), resolveSession),
		registration("Request", allowed, matchNamedType[Request]("Request"), resolveRequest),
		registration("Response", allowed, matchNamedType[Response]("Response"), resolveResponse),
		// 面向业务结构体参数的两类兜底匹配：
		// - AutoStruct：普通字段按名称自动绑定
		// - CompositeStruct：字段中混入显式 binding wrapper
		registration("AutoStruct", allowed, matchAutoStruct, resolveAutoStruct),
		registration("CompositeStruct", allowed, matchCompositeStruct, resolveCompositeStruct),
	}
}

// DefaultResolvers 返回 GraphQL 协议默认启用的一组 binding resolver。
func DefaultResolvers() []runtime.BindingResolver {
	return runtime.NewBindingResolvers(DefaultRegistrations()...)
}

// registration 是一个本地薄封装，目的是让默认表更容易扫读。
func registration(name string, allowed map[runtime.Protocol]bool, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:             name,
		AllowedProtocols: allowed,
		Match:            match,
		Resolve:          resolve,
	}
}
