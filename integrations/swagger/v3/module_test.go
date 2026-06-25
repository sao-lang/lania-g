package swagger

import (
	stdctx "context"
	"reflect"
	"strings"
	"testing"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	bindinghttp "github.com/sao-lang/lania-g/protocol/http/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

type swaggerUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type swaggerErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func TestForRoot_RegistersBuilderFactoryAndConfig(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{Title: "Demo API", Version: "1.0.0"})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	builderToken := reflect.TypeFor[*Builder]()
	factoryToken := reflect.TypeFor[Factory]()
	uiConfigToken := reflect.TypeFor[*UIConfig]()

	builderAny, err := mod.Container().Get(builderToken)
	if err != nil {
		t.Fatalf("get builder: %v", err)
	}
	builder := builderAny.(*Builder)
	builder.AddSchemaFromType("User", swaggerUser{})
	body, err := builder.ToJSON()
	if err != nil {
		t.Fatalf("to json: %v", err)
	}
	if !strings.Contains(string(body), "Demo API") {
		t.Fatalf("spec=%s", string(body))
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	if _, err := br.Resolve(ctx, nil, builderToken, 0); err != nil {
		t.Fatalf("resolve builder: %v", err)
	}
	if _, err := br.Resolve(ctx, nil, uiConfigToken, 1); err != nil {
		t.Fatalf("resolve ui config: %v", err)
	}
	factoryAny, err := br.Resolve(ctx, nil, factoryToken, 2)
	if err != nil {
		t.Fatalf("resolve factory: %v", err)
	}
	derived, err := factoryAny.(Factory).New(Config{Title: "Another"})
	if err != nil || derived == nil {
		t.Fatalf("derived builder: %v", err)
	}
}

type swaggerAutoController struct{}

type swaggerCreateUserRequest struct {
	Name   string            `json:"name" description:"user name" example:"alice"`
	Role   string            `json:"role" enum:"admin|guest" default:"guest"`
	Tags   []string          `json:"tags,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Secret string            `json:"secret,omitempty" swagger:"-"`
}

func (c *swaggerAutoController) GetUser() error { return nil }

type swaggerQueryArgs struct {
	ID    bindinghttp.Param[string]  `param:"id" required:"true" description:"user id"`
	Trace bindinghttp.Header[string] `header:"X-Trace-ID" description:"trace id"`
}

type swaggerCreateArgs struct {
	Body  bindinghttp.Body[swaggerCreateUserRequest]
	Trace bindinghttp.Header[string] `header:"X-Trace-ID"`
}

type swaggerCreateUserResponse struct {
	ID   int      `json:"id"`
	Tags []string `json:"tags"`
}

type swaggerEnvelope struct {
	Data interface{} `json:"data"`
	Meta struct {
		RequestID string `json:"requestId"`
	} `json:"meta"`
}

func (c *swaggerAutoController) QueryUser(args swaggerQueryArgs) (swaggerUser, error) {
	return swaggerUser{}, nil
}
func (c *swaggerAutoController) CreateUser(args swaggerCreateArgs) (swaggerCreateUserResponse, error) {
	return swaggerCreateUserResponse{}, nil
}

func TestBuildFromHTTPRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	ctrl := &swaggerAutoController{}
	httpadapter.Controller("/users", ctrl).
		Get("/:id", ctrl.QueryUser).
		Post("/", ctrl.CreateUser).
		Summary("Create User").
		Description("Create a new user").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(swaggerEnvelope{}, "data").
		ErrorResponse(409, "Conflict").
		Build()

	builder, err := New(Config{Title: "Auto API"})
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}
	builder.AddBearerAuth("bearerAuth")
	builder.SetDefaultErrorResponse("Request failed", swaggerErrorResponse{}, 400, 500)
	if _, err := BuildFromHTTPRegistry(builder, registry.Global()); err != nil {
		t.Fatalf("build from registry: %v", err)
	}
	spec := builder.Build()
	userPath := spec.Paths["/users/:id"]
	if userPath == nil || userPath.Get == nil {
		t.Fatalf("missing get path: %+v", spec.Paths)
	}
	if len(userPath.Get.Parameters) != 2 || userPath.Get.Parameters[0].Name != "id" {
		t.Fatalf("params=%+v", userPath.Get.Parameters)
	}
	if userPath.Get.Responses["200"] == nil || userPath.Get.Responses["200"].Content["application/json"] == nil {
		t.Fatalf("missing get response schema: %+v", userPath.Get.Responses)
	}
	createPath := spec.Paths["/users/"]
	if createPath == nil || createPath.Post == nil || createPath.Post.RequestBody == nil {
		t.Fatalf("missing post request body: %+v", spec.Paths)
	}
	if createPath.Post.Summary != "Create User" || createPath.Post.Description != "Create a new user" {
		t.Fatalf("summary/description=%+v", createPath.Post)
	}
	if len(createPath.Post.Tags) != 1 || createPath.Post.Tags[0] != "users" {
		t.Fatalf("tags=%+v", createPath.Post.Tags)
	}
	if len(createPath.Post.Security) != 1 || createPath.Post.Security[0]["bearerAuth"] == nil {
		t.Fatalf("security=%+v", createPath.Post.Security)
	}
	bodySchema := createPath.Post.RequestBody.Content["application/json"].Schema
	if bodySchema == nil || bodySchema.Properties["role"] == nil || len(bodySchema.Properties["role"].Enum) != 2 {
		t.Fatalf("missing enum role schema: %+v", bodySchema)
	}
	if bodySchema.Properties["labels"] == nil || bodySchema.Properties["labels"].AdditionalProperties == nil {
		t.Fatalf("missing map schema: %+v", bodySchema.Properties["labels"])
	}
	if _, ok := bodySchema.Properties["secret"]; ok {
		t.Fatalf("ignored field still exists: %+v", bodySchema.Properties)
	}
	if len(createPath.Post.Parameters) != 1 || createPath.Post.Parameters[0].Name != "X-Trace-ID" {
		t.Fatalf("post params=%+v", createPath.Post.Parameters)
	}
	respSchema := createPath.Post.Responses["200"].Content["application/json"].Schema
	if respSchema == nil || respSchema.Properties["data"] == nil {
		t.Fatalf("response envelope=%+v", respSchema)
	}
	if createPath.Post.Responses["409"] == nil || createPath.Post.Responses["500"] == nil {
		t.Fatalf("error responses=%+v", createPath.Post.Responses)
	}
}

func TestBuildFromHTTPRegistry_RequiresExplicitRegistry(t *testing.T) {
	builder, err := New(Config{Title: "Auto API"})
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}
	if _, err := BuildFromHTTPRegistry(builder, nil); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildFromHTTPRegistryCompat(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	ctrl := &swaggerAutoController{}
	httpadapter.Controller("/compat-users", ctrl).
		Get("/:id", ctrl.QueryUser).
		Build()

	builder, err := New(Config{Title: "Compat API"})
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}
	BuildFromHTTPRegistryCompat(builder)
	spec := builder.Build()
	if spec.Paths["/compat-users/:id"] == nil || spec.Paths["/compat-users/:id"].Get == nil {
		t.Fatalf("missing compat get path: %+v", spec.Paths)
	}
}

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{Title: "Demo API"})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	if err := mod.Init(); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForRootCompat_InitRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRootCompat(Config{Title: "Compat API"})
	if err != nil {
		t.Fatalf("for root compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/swagger.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
