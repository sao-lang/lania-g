// builder.go 实现 swagger 集成对外暴露的构建器与文档组装逻辑。
package swagger

import (
	"encoding/json"
	"reflect"
)

// Config 描述生成 OpenAPI 文档时的基础信息。
type Config struct {
	Title       string
	Description string
	Version     string
}

// UIConfig 描述 Swagger UI / Redoc 页面使用的展示配置。
type UIConfig struct {
	Title      string
	SpecURL    string
	SwaggerURL string
	RedocURL   string
}

// Factory 约定 swagger builder 工厂的最小能力。
type Factory interface {
	Default() *Builder
	New(cfg Config) (*Builder, error)
}

// Builder 用于以编程方式构建 OpenAPI 文档。
type Builder struct {
	openapi             *OpenAPI
	defaultErrorSchema  *Schema
	defaultErrorMessage string
	defaultErrorCodes   []int
}

// New 创建一个 swagger Builder，并填充默认文档信息。
func New(cfg Config) (*Builder, error) {
	if cfg.Title == "" {
		cfg.Title = "API Documentation"
	}
	if cfg.Description == "" {
		cfg.Description = "API Documentation"
	}
	if cfg.Version == "" {
		cfg.Version = "1.0.0"
	}
	return &Builder{
		openapi: &OpenAPI{
			OpenAPIVersion: "3.0.3",
			Info: &Info{
				Title:       cfg.Title,
				Description: cfg.Description,
				Version:     cfg.Version,
			},
			Paths: make(map[string]*Path),
			Components: &Components{
				Schemas:         make(map[string]*Schema),
				SecuritySchemes: make(map[string]*SecurityScheme),
			},
			Tags:    make([]*Tag, 0),
			Servers: make([]*Server, 0),
		},
	}, nil
}

// DefaultUIConfig 返回 Swagger UI / Redoc 的默认展示配置。
func DefaultUIConfig() *UIConfig {
	return &UIConfig{
		Title:      "API Documentation",
		SpecURL:    "/swagger.json",
		SwaggerURL: "/swagger",
		RedocURL:   "/redoc",
	}
}

// Default 返回当前 builder 本身，便于满足 Factory 风格接口。
func (b *Builder) Default() *Builder { return b }

// New 以工厂风格创建一个新的 Builder。
func (b *Builder) New(cfg Config) (*Builder, error) { return New(cfg) }

// SetInfo 设置 OpenAPI 文档顶层的标题、描述和版本号。
func (b *Builder) SetInfo(title, description, version string) *Builder {
	b.openapi.Info.Title = title
	b.openapi.Info.Description = description
	b.openapi.Info.Version = version
	return b
}

// AddServer 向文档追加一个可访问的服务端点。
func (b *Builder) AddServer(url, description string) *Builder {
	b.openapi.Servers = append(b.openapi.Servers, &Server{URL: url, Description: description})
	return b
}

// AddTag 向文档追加一个标签分组。
func (b *Builder) AddTag(name, description string) *Builder {
	b.openapi.Tags = append(b.openapi.Tags, &Tag{Name: name, Description: description})
	return b
}

// AddBearerAuth 注册一个 Bearer/JWT 认证方案。
func (b *Builder) AddBearerAuth(name string) *Builder {
	b.openapi.Components.SecuritySchemes[name] = &SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
	}
	return b
}

// AddOAuth2 注册一个 OAuth2 认证方案。
func (b *Builder) AddOAuth2(name string, flows *OAuthFlows) *Builder {
	b.openapi.Components.SecuritySchemes[name] = &SecurityScheme{Type: "oauth2", Flows: flows}
	return b
}

// AddPath 直接写入一条 OpenAPI Path 定义。
func (b *Builder) AddPath(path string, pathItem *Path) *Builder {
	b.openapi.Paths[path] = pathItem
	return b
}

// AddSchema 向 components 中注册一个可复用 schema。
func (b *Builder) AddSchema(name string, schema *Schema) *Builder {
	b.openapi.Components.Schemas[name] = schema
	return b
}

// AddSchemaFromType 根据 Go 类型推导并注册一个 schema。
func (b *Builder) AddSchemaFromType(name string, typ interface{}) *Builder {
	b.openapi.Components.Schemas[name] = b.reflectSchema(typ)
	return b
}

// SetDefaultErrorResponse 设置默认错误响应的描述、schema 与状态码集合。
func (b *Builder) SetDefaultErrorResponse(description string, schema interface{}, statusCodes ...int) *Builder {
	if description == "" {
		description = "Error"
	}
	b.defaultErrorMessage = description
	if schema != nil {
		b.defaultErrorSchema = b.reflectSchema(schema)
	}
	if len(statusCodes) == 0 {
		statusCodes = []int{400, 500}
	}
	b.defaultErrorCodes = append([]int{}, statusCodes...)
	return b
}

func (b *Builder) defaultErrorResponse() *Response {
	resp := &Response{Description: b.defaultErrorMessage}
	if resp.Description == "" {
		resp.Description = "Error"
	}
	if b.defaultErrorSchema != nil {
		resp.Content = map[string]*MediaType{
			"application/json": {Schema: b.defaultErrorSchema},
		}
	}
	return resp
}

// Build 返回当前累积构造出的 OpenAPI 文档对象。
func (b *Builder) Build() *OpenAPI { return b.openapi }

// ToJSON 将当前 OpenAPI 文档序列化为格式化 JSON。
func (b *Builder) ToJSON() ([]byte, error) { return json.MarshalIndent(b.openapi, "", "  ") }

func (b *Builder) reflectSchema(typ interface{}) *Schema {
	t := reflect.TypeOf(typ)
	return inferSchema(t)
}

// GenerateSwaggerUIHTML 生成可直接返回给浏览器的 Swagger UI HTML 页面。
func GenerateSwaggerUIHTML(config *UIConfig) string {
	if config == nil {
		config = DefaultUIConfig()
	}
	return `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><title>` + config.Title + `</title><link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" /></head><body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script><script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js"></script><script>window.onload = function(){window.ui = SwaggerUIBundle({url:"` + config.SpecURL + `",dom_id:'#swagger-ui',deepLinking:true,presets:[SwaggerUIBundle.presets.apis,SwaggerUIStandalonePreset],layout:"StandaloneLayout"});};</script></body></html>`
}

// GenerateRedocHTML 生成可直接返回给浏览器的 Redoc HTML 页面。
func GenerateRedocHTML(config *UIConfig) string {
	if config == nil {
		config = DefaultUIConfig()
	}
	return `<!DOCTYPE html><html><head><title>` + config.Title + `</title><meta charset="utf-8"/><meta name="viewport" content="width=device-width, initial-scale=1"></head><body><redoc spec-url="` + config.SpecURL + `"></redoc><script src="https://unpkg.com/redoc@2.1.3/bundles/redoc.standalone.js"></script></body></html>`
}
