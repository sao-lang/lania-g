// auto.go 实现 swagger 集成的自动发现与自动生成总入口。
package swagger

import (
	"fmt"
	"reflect"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// BuildFromHTTPRegistry 会从 HTTP registry 声明中自动生成 Swagger 文档内容。
// 推荐：显式传入应用实例使用的 registry。
func BuildFromHTTPRegistry(builder *Builder, reg *registry.Registry) (*Builder, error) {
	if reg == nil {
		return nil, fmt.Errorf("swagger registry build requires an explicit registry; pass the application registry instance or call BuildFromHTTPRegistryCompat(...) explicitly")
	}
	return buildFromHTTPRegistry(builder, reg), nil
}

// BuildFromHTTPRegistryCompat 会从全局 HTTP registry 声明中自动生成 Swagger 文档内容。
func BuildFromHTTPRegistryCompat(builder *Builder) *Builder {
	return buildFromHTTPRegistry(builder, registry.Global())
}

func buildFromHTTPRegistry(builder *Builder, reg *registry.Registry) *Builder {
	if builder == nil {
		return nil
	}
	builderContext = builder
	defer func() { builderContext = nil }()
	items := reg.ListDecl(httpadapter.AdapterID, "routes")
	for _, item := range items {
		def, ok := item.(*httpadapter.RouteDefinition)
		if !ok || def == nil {
			continue
		}
		builder.AddHTTPRoute(def)
	}
	return builder
}

// AddHTTPRoute 把一条 HTTP 路由定义转换为 OpenAPI path/operation。
func (b *Builder) AddHTTPRoute(def *httpadapter.RouteDefinition) *Builder {
	if b == nil || def == nil {
		return b
	}
	if def.Doc != nil && def.Doc.Hidden {
		return b
	}
	path := ensurePath(def.Path)
	item := b.openapi.Paths[path]
	if item == nil {
		item = &Path{}
		b.openapi.Paths[path] = item
	}
	op := &Operation{
		OperationID: httpOperationID(def),
		Summary:     routeSummary(def),
		Description: routeDescription(def),
		Responses:   responseSpecForRoute(def),
	}
	op.Tags = routeTags(def)
	op.Security = routeSecurity(def)
	op.Parameters = append(op.Parameters, parametersForRoute(def)...)
	if body := requestBodyForRoute(def); body != nil {
		op.RequestBody = body
	}
	assignOperation(item, def.Method, op)
	return b
}

func isErrorType(t reflect.Type) bool {
	return t != nil && t.Implements(reflect.TypeFor[error]())
}

var builderContext *Builder

func currentBuilder() *Builder { return builderContext }
