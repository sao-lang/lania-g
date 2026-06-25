// register.go 注册 HTTP 协议的默认 binding 声明与 compat 入口。
package http

import (
	"reflect"

	coreregistry "github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpprotocol "github.com/sao-lang/lania-g/protocol/http/v3/protocol"
)

// 这组 metadata key 是 http adapter 与 binding/http 之间的协作约定：
// adapter 在接入层先把请求期对象写入 HandlerContext.Metadata，
// resolver 再按这些固定 key 读取对应值。
const (
	MetadataKeyNext              = "http.next"
	MetadataKeyContext           = "http.context"
	MetadataKeyRenderer          = "http.renderer"
	MetadataKeyValidator         = "http.validator"
	MetadataKeyWritten           = "http.response.written"
	MetadataKeyForm              = "http.form"
	MetadataKeyCookies           = "http.cookies"
	MetadataKeySession           = "http.session"
	MetadataKeyAuthUser          = "http.auth.user"
	MetadataKeyAuthUserID        = "http.auth.user_id"
	MetadataKeyAuthToken         = "http.auth.token"
	MetadataKeyAuthOptionalUser  = "http.auth.optional_user"
	MetadataKeyAuthOptionalToken = "http.auth.optional_token"
	MetadataKeyFiles             = "http.files"
)

// RegisterDefaults 将内置的 HTTP 参数绑定包装类型注册到 runtime。
//
// 注意：runtime 不会自动调用它；需要 adapter/application 显式选择启用。
func RegisterDefaults(rt *runtime.Runtime) {
	for _, reg := range DefaultRegistrations() {
		rt.RegisterBinding(runtime.NewBindingResolver(reg))
	}
}

// RegisterDefaultsToRegistry 将内置 HTTP binding resolver 注册到 registry。
func RegisterDefaultsToRegistry(reg *coreregistry.Registry) {
	if reg == nil {
		RegisterDefaultsCompat()
		return
	}
	registerDefaultsToRegistry(reg)
}

// RegisterDefaultsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterDefaultsCompat() {
	registerDefaultsToRegistry(coreregistry.GlobalWithUsage("binding/http.RegisterDefaultsCompat"))
}

func registerDefaultsToRegistry(reg *coreregistry.Registry) {
	reg.RegisterBindings(DefaultResolvers()...)
}

// DefaultRegistrations 返回内置的 HTTP binding registration 列表。
//
// 顺序上大致分三层：
// - 上下文类注入：`HandlerContext`、`HttpContext`
// - 显式 wrapper：`Body[T]`、`Query[T]`、`Header[T]`、鉴权 wrapper 等
// - 兜底 struct 绑定：`AutoStruct`、`CompositeStruct`
//
// 最后的兜底项必须放在 wrapper 之后，避免把本来应该按显式语义处理的参数
// 提前吞成“普通 struct 自动绑定”。
func DefaultRegistrations() []runtime.BindingRegistration {
	allowed := map[runtime.Protocol]bool{httpprotocol.Protocol: true}
	return []runtime.BindingRegistration{
		registration("HandlerContext", nil, matchHandlerContext, resolveHandlerContext),
		registration("HttpContext", allowed, matchHTTPContext, resolveHTTPContext),
		registration("Body", allowed, matchGenericWrapper("Body"), resolveBody),
		registration("Query", allowed, matchGenericWrapper("Query"), resolveQuery),
		registration("Param", allowed, matchGenericWrapper("Param"), resolveParam),
		registration("Header", allowed, matchGenericWrapper("Header"), resolveHeader),
		registration("Form", allowed, matchGenericWrapper("Form"), resolveForm),
		registration("Bind", allowed, matchGenericWrapper("Bind"), resolveBind),
		registration("MustBind", allowed, matchGenericWrapper("MustBind"), resolveMustBind),
		registration("BodyBytes", allowed, matchNamedType[BodyBytes]("BodyBytes"), resolveBodyBytes),
		registration("MustBodyBytes", allowed, matchNamedType[MustBodyBytes]("MustBodyBytes"), resolveBodyBytes),
		registration("BodyAs", allowed, matchGenericWrapper("BodyAs"), resolveBody),
		registration("MustBodyAs", allowed, matchGenericWrapper("MustBodyAs"), resolveMustBody),
		registration("Original", allowed, matchNamedType[Original]("Original"), resolveOriginal),
		registration("Next", allowed, matchNamedType[Next]("Next"), resolveNext),
		registration("IP", allowed, matchNamedType[IP]("IP"), resolveIP),
		registration("Host", allowed, matchNamedType[Host]("Host"), resolveHost),
		registration("Method", allowed, matchNamedType[Method]("Method"), resolveMethod),
		registration("Path", allowed, matchNamedType[Path]("Path"), resolvePath),
		registration("URL", allowed, matchNamedType[URL]("URL"), resolveURL),
		registration("Headers", allowed, matchNamedType[Headers]("Headers"), resolveHeaders),
		registration("Session", allowed, matchNamedType[Session]("Session"), resolveSession),
		registration("File", allowed, matchNamedType[File]("File"), resolveFile),
		registration("Files", allowed, matchNamedType[Files]("Files"), resolveFiles),
		registration("Cookie", allowed, matchGenericWrapper("Cookie"), resolveCookie),
		registration("Cookies", allowed, matchNamedType[Cookies]("Cookies"), resolveCookies),
		registration("AuthUser", allowed, matchNamedType[AuthUser]("AuthUser"), resolveAuthUser),
		registration("AuthUserID", allowed, matchNamedType[AuthUserID]("AuthUserID"), resolveAuthUserID),
		registration("AuthToken", allowed, matchNamedType[AuthToken]("AuthToken"), resolveAuthToken),
		registration("AuthOptionalUser", allowed, matchNamedType[AuthOptionalUser]("AuthOptionalUser"), resolveAuthOptionalUser),
		registration("AuthOptionalToken", allowed, matchNamedType[AuthOptionalToken]("AuthOptionalToken"), resolveAuthOptionalToken),
		registration("AuthRole", allowed, matchNamedType[AuthRole]("AuthRole"), resolveAuthRole),
		registration("AuthPermission", allowed, matchNamedType[AuthPermission]("AuthPermission"), resolveAuthPermission),
		registration("AutoStruct", allowed, matchAutoStruct, resolveAutoStruct),
		registration("CompositeStruct", allowed, matchCompositeStruct, resolveCompositeStruct),
	}
}

// DefaultResolvers 返回内置的 HTTP binding resolver 列表。
func DefaultResolvers() []runtime.BindingResolver {
	return runtime.NewBindingResolvers(DefaultRegistrations()...)
}

// registration 是一个本地薄封装，用来避免每次都手写同样的结构体字面量。
// 它本身不带额外行为，只负责把 matcher/resolve 成对打包。
func registration(name string, allowed map[runtime.Protocol]bool, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:             name,
		AllowedProtocols: allowed,
		Match:            match,
		Resolve:          resolve,
	}
}
