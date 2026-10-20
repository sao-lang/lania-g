package graphql_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/protocol/graphql/v3"
import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// 下面的模型用于验证"未显式声明 Returns 时按 Go 类型推断 schema"的行为。

type InferBase struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Success bool   `json:"success"`
}

type InferAccount struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type InferResult struct {
	InferBase
	Account InferAccount `json:"account"`
}

type InferSelfRef struct {
	Name    string        `json:"name"`
	Manager *InferSelfRef `json:"manager"`
}

type InferSkip struct {
	Visible string `json:"visible"`
	Hidden  string `json:"-"`
}

type inferResolver struct{}

func (r *inferResolver) Result() (InferResult, error) {
	return InferResult{
		InferBase: InferBase{Code: 0, Msg: "ok", Success: true},
		Account:   InferAccount{ID: 7, Username: "alice"},
	}, nil
}

func (r *inferResolver) Self() (InferSelfRef, error) {
	return InferSelfRef{Name: "root", Manager: &InferSelfRef{Name: "boss"}}, nil
}

func (r *inferResolver) Skip() (InferSkip, error) {
	return InferSkip{Visible: "yes", Hidden: "no"}, nil
}

// buildInferAdapter 构造一个只依赖类型推断的 GraphQL adapter。
func buildInferAdapter(t *testing.T) *Adapter {
	t.Helper()

	resolver := &inferResolver{}
	root := module.New(module.Metadata{Resolvers: []any{resolver}})
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)
	reg := registry.New()

	routes := NewRoutes(reg, resolver)
	routes.Resolver("Query").
		Query("result", resolver.Result).
		Query("self", resolver.Self).
		Query("skip", resolver.Skip)
	if err := routes.Err(); err != nil {
		t.Fatalf("declare routes: %v", err)
	}

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil {
		t.Fatalf("install: %v", err)
	}

	a := New()
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return a
}

func queryGraphQL(t *testing.T, a *Adapter, query string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/graphql",
		strings.NewReader(`{"query":`+strconvQuote(query)+`}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func strconvQuote(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			builder.WriteString(`\"`)
		case '\\':
			builder.WriteString(`\\`)
		case '\n':
			builder.WriteString(`\n`)
		default:
			builder.WriteRune(r)
		}
	}
	builder.WriteByte('"')
	return builder.String()
}

func TestModelInference_FlattensEmbeddedStruct(t *testing.T) {
	a := buildInferAdapter(t)

	code, body := queryGraphQL(t, a, `{ result { code msg success account { id username } } }`)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}
	for _, want := range []string{`"code":0`, `"msg":"ok"`, `"success":true`, `"account":{"id":7,"username":"alice"}`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %s: %s", want, body)
		}
	}
}

func TestModelInference_SkipsJSONDash(t *testing.T) {
	a := buildInferAdapter(t)

	code, body := queryGraphQL(t, a, `{ skip { visible } }`)
	if code != http.StatusOK || !strings.Contains(body, `"visible":"yes"`) {
		t.Fatalf("status=%d body=%s", code, body)
	}

	code, body = queryGraphQL(t, a, `{ skip { hidden } }`)
	if code == http.StatusOK && strings.Contains(body, `"hidden":"no"`) {
		t.Fatalf("json:\"-\" field must not be inferable: %s", body)
	}
}

func TestModelInference_SelfReference(t *testing.T) {
	a := buildInferAdapter(t)

	code, body := queryGraphQL(t, a, `{ self { name manager { name } } }`)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%s", code, body)
	}
	if !strings.Contains(body, `"manager":{"name":"boss"}`) {
		t.Fatalf("self reference not resolved: %s", body)
	}
}

func TestModelInference_ImplicitFlattenedField(t *testing.T) {
	a := buildInferAdapter(t)

	code, body := queryGraphQL(t, a, `{ result { msg } }`)
	if code != http.StatusOK || !strings.Contains(body, `"msg":"ok"`) {
		t.Fatalf("status=%d body=%s", code, body)
	}
}
