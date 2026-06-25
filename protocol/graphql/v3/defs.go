// defs.go 定义 GraphQL adapter 在 registry 与编译阶段使用的声明结构。
package graphql

// FieldType 表示 GraphQL 字段类型（Query/Mutation/Object/Subscription）。
type FieldType string

const (
	// FieldTypeQuery 表示查询字段。
	FieldTypeQuery        FieldType = "Query"
	// FieldTypeMutation 表示变更字段。
	FieldTypeMutation     FieldType = "Mutation"
	// FieldTypeSubscription 表示订阅字段。
	FieldTypeSubscription FieldType = "Subscription"
	// FieldTypeObject 表示对象字段（用于嵌套选择集的分发）。
	FieldTypeObject       FieldType = "Object"
)

// RateLimitConfig 描述简单的限流配置。
type RateLimitConfig struct {
	Limit  int
	Period int64
}

// ResolverDefinition 表示一个 resolver 的编译期声明。
type ResolverDefinition struct {
	Name         string
	Resolver     any
	Fields       []*FieldDefinition
	Guards       []any
	Interceptors []any
	Middlewares  []any
	Pipes        []any
	ParamPipes   map[int][]any
	Filters      []any
}

// FieldDefinition 表示 resolver 上某个字段的编译期声明。
type FieldDefinition struct {
	FieldType   FieldType
	FieldName   string
	Handler     any
	HandlerName string
	// Returns 表示该字段返回对象时对应的 GraphQL 类型名。
	// adapter 会用它来把嵌套选择集分发给对应的 Object 字段 resolver。
	Returns      string
	Description  string
	Deprecation  string
	Args         []*ArgDefinition
	Guards       []any
	Interceptors []any
	Middlewares  []any
	Pipes        []any
	ParamPipes   map[int][]any
	Filters      []any
	CacheControl string
	Permissions  []string
	Timeout      int64
	Complexity   int
	RateLimit    *RateLimitConfig
}

// ArgDefinition 表示字段参数的编译期声明。
type ArgDefinition struct {
	Name         string
	Description  string
	Required     bool
	DefaultValue any
}

// Schema 描述一份 GraphQL schema 配置（Query/Mutation/Object 结构）。
type Schema struct {
	Query       *ObjectSchema
	Mutation    *ObjectSchema
	Objects     map[string]*ObjectSchema
	ScalarNames map[string]bool
}

// ObjectSchema 描述一个 GraphQL object type 的字段集合。
type ObjectSchema struct {
	Name   string
	Fields map[string]*FieldSchema
}

// FieldSchema 描述 object type 下某个字段的类型与文档信息。
type FieldSchema struct {
	Name         string
	TypeName     string
	List         bool
	NonNull      bool
	Description  string
	Deprecation  string
	Args         map[string]*ArgSchema
	Complexity   int
	CacheControl string
}

// ArgSchema 描述字段参数的类型信息。
type ArgSchema struct {
	Name         string
	TypeName     string
	List         bool
	NonNull      bool
	Description  string
	DefaultValue any
}

// Extension 定义 GraphQL 扩展钩子。
type Extension interface {
	BeforeOperation(*OperationContext) error
	AfterOperation(*OperationContext, *GraphQLResponse)
}

// ConfigDecl 表示写入 registry 的 GraphQL adapter 配置声明。
type ConfigDecl struct {
	Schema               *Schema
	DisableIntrospection bool
	ComplexityLimit      int
	Extensions           []Extension
}
