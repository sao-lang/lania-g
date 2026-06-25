// types.go 定义 swagger 集成对外暴露的公共类型、选项与包装结构。
package swagger

// OpenAPI 表示一份完整的 OpenAPI 3 文档。
type OpenAPI struct {
	OpenAPIVersion string           `json:"openapi,omitempty"`
	Info           *Info            `json:"info,omitempty"`
	Paths          map[string]*Path `json:"paths,omitempty"`
	Components     *Components      `json:"components,omitempty"`
	Tags           []*Tag           `json:"tags,omitempty"`
	Servers        []*Server        `json:"servers,omitempty"`
}

// Info 描述文档顶层的基础信息。
type Info struct {
	Title          string   `json:"title,omitempty"`
	Description    string   `json:"description,omitempty"`
	TermsOfService string   `json:"termsOfService,omitempty"`
	Contact        *Contact `json:"contact,omitempty"`
	License        *License `json:"license,omitempty"`
	Version        string   `json:"version,omitempty"`
}

// Contact 描述 API 联系人信息。
type Contact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

// License 描述 API 使用许可信息。
type License struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

// Tag 描述 OpenAPI 中的一个标签分组。
type Tag struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// Server 描述一个可访问的服务端点。
type Server struct {
	URL         string                     `json:"url,omitempty"`
	Description string                     `json:"description,omitempty"`
	Variables   map[string]*ServerVariable `json:"variables,omitempty"`
}

// ServerVariable 描述 Server URL 模板中的变量。
type ServerVariable struct {
	Enum        []string `json:"enum,omitempty"`
	Default     string   `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
}

// Path 描述某个路径上的一组 HTTP 操作。
type Path struct {
	Summary     string       `json:"summary,omitempty"`
	Description string       `json:"description,omitempty"`
	Get         *Operation   `json:"get,omitempty"`
	Put         *Operation   `json:"put,omitempty"`
	Post        *Operation   `json:"post,omitempty"`
	Delete      *Operation   `json:"delete,omitempty"`
	Options     *Operation   `json:"options,omitempty"`
	Head        *Operation   `json:"head,omitempty"`
	Patch       *Operation   `json:"patch,omitempty"`
	Trace       *Operation   `json:"trace,omitempty"`
	Parameters  []*Parameter `json:"parameters,omitempty"`
}

// Operation 描述一个具体的接口操作。
type Operation struct {
	Tags        []string              `json:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	OperationID string                `json:"operationId,omitempty"`
	Parameters  []*Parameter          `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]*Response  `json:"responses,omitempty"`
	Security    []map[string][]string `json:"security,omitempty"`
}

// Parameter 描述一个接口参数。
type Parameter struct {
	Name        string      `json:"name,omitempty"`
	In          string      `json:"in,omitempty"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Schema      *Schema     `json:"schema,omitempty"`
	Example     interface{} `json:"example,omitempty"`
}

// RequestBody 描述接口请求体。
type RequestBody struct {
	Description string                `json:"description,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
	Required    bool                  `json:"required,omitempty"`
}

// MediaType 描述某种内容类型对应的数据结构。
type MediaType struct {
	Schema   *Schema     `json:"schema,omitempty"`
	Example  interface{} `json:"example,omitempty"`
	Examples interface{} `json:"examples,omitempty"`
}

// Response 描述一个接口响应。
type Response struct {
	Description string                `json:"description,omitempty"`
	Headers     map[string]*Header    `json:"headers,omitempty"`
	Content     map[string]*MediaType `json:"content,omitempty"`
}

// Header 描述响应头或参数头的结构。
type Header struct {
	Description string      `json:"description,omitempty"`
	Schema      *Schema     `json:"schema,omitempty"`
	Example     interface{} `json:"example,omitempty"`
}

// Schema 描述 OpenAPI 中的数据模型结构。
type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	Description          string             `json:"description,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	Ref                  string             `json:"$ref,omitempty"`
	Example              interface{}        `json:"example,omitempty"`
	Default              interface{}        `json:"default,omitempty"`
	Enum                 []interface{}      `json:"enum,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	Nullable             bool               `json:"nullable,omitempty"`
}

// Components 汇总可复用的 schema 与安全定义。
type Components struct {
	Schemas         map[string]*Schema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitempty"`
}

// SecurityScheme 描述一种安全认证方案。
type SecurityScheme struct {
	Type             string      `json:"type,omitempty"`
	Scheme           string      `json:"scheme,omitempty"`
	BearerFormat     string      `json:"bearerFormat,omitempty"`
	Flows            *OAuthFlows `json:"flows,omitempty"`
	OpenIDConnectURL string      `json:"openIdConnectUrl,omitempty"`
}

// OAuthFlows 描述 OAuth2 的多种授权流程集合。
type OAuthFlows struct {
	Implicit          *OAuthFlow `json:"implicit,omitempty"`
	Password          *OAuthFlow `json:"password,omitempty"`
	ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty"`
}

// OAuthFlow 描述单个 OAuth2 授权流程。
type OAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	RefreshURL       string            `json:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes,omitempty"`
}
