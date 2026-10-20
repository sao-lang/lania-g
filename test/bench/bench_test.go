// Package bench 是框架的性能基线场景集。
//
// 这里的 benchmark 刻意覆盖四个"最容易退化"的路径：
//
//	BenchHTTP_StaticJSON   最小 HTTP 链路（路由匹配 + 执行 + JSON 序列化）
//	BenchHTTP_AOPDepth10   10 层中间件，验证链式无分配放大
//	BenchHTTP_BindValidate Body 解码 + validator，验证 binding 层成本
//	BenchDI_ResolveNested  5 层依赖解析，验证容器性能
//
// 基线文件在 test/bench/baseline.txt，用 tools/benchcheck 做回归门禁。
//
// 基线必须「每项 benchmark 独立进程采集」，因为同一进程内后跑的 benchmark 会受
// 前面 benchmark 留下的堆/GC 状态影响（实测漂移 10%~20%，足以让门禁误报）：
//
//	cd test
//	for b in BenchmarkHTTP_StaticJSON BenchmarkHTTP_AOPDepth10 BenchmarkHTTP_BindValidate BenchmarkDI_ResolveNested; do
//		go test -run XXX -bench "^$b$" -benchmem -count 6 ./bench/ | grep '^Benchmark'
//	done > bench/baseline.txt
//
//	cd ../tools && go run ./cmd/benchcheck -baseline ../test/bench/baseline.txt -current <new.txt>
package bench_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	application "github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
)

// ---------------------------------------------------------------------------
// HTTP 场景
// ---------------------------------------------------------------------------

type benchController struct{}

type benchUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type benchCreateReq struct {
	Name  string `json:"name" validate:"required,min=3"`
	Email string `json:"email" validate:"required,email"`
}

func (c *benchController) Get() (benchUser, error) {
	return benchUser{ID: 1, Name: "alice"}, nil
}

func (c *benchController) Create(body httpbinding.Body[benchCreateReq]) (benchUser, error) {
	return benchUser{ID: 2, Name: body.Value.Name}, nil
}

func (c *benchController) Declare(r *httpadapter.Routes) {
	r.Get("/users/1", c.Get)
	r.Post("/users", c.Create)
}

// buildHTTPHandler 编译一个最小应用，并返回可调用的 HTTP handler。
//
// 不做 Start（不监听端口），因此 benchmark 只度量框架开销，不含网络栈。
func buildHTTPHandler(b *testing.B, middlewares ...aop.Middleware) http.Handler {
	b.Helper()
	root := module.New(module.Metadata{
		Imports: []module.Module{
			httpadapter.Module(httpadapter.Config{Validator: true}),
		},
		Controllers: []any{&benchController{}},
	})
	app, err := application.New(root, application.Options{})
	if err != nil {
		b.Fatalf("new application: %v", err)
	}
	if len(middlewares) > 0 {
		app.UseGlobalMiddlewares(middlewares...)
	}
	if _, err := app.StartupReport(); err != nil {
		b.Fatalf("compile: %v", err)
	}
	adp, ok := app.Adapter("http")
	if !ok {
		b.Fatalf("http adapter not mounted")
	}
	handler, ok := adp.(http.Handler)
	if !ok {
		b.Fatalf("http adapter is not an http.Handler")
	}
	return handler
}

func noopMiddleware(ctx *aop.ExecutionContext, next func() error) error { return next() }

// BenchmarkHTTP_StaticJSON 衡量"静态路由 + 返回 struct + JSON 序列化"的完整开销。
func BenchmarkHTTP_StaticJSON(b *testing.B) {
	handler := buildHTTPHandler(b)
	req := httptest.NewRequest(http.MethodGet, "http://bench.local/users/1", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

// BenchmarkHTTP_AOPDepth10 衡量 10 层中间件的叠加成本。
func BenchmarkHTTP_AOPDepth10(b *testing.B) {
	middlewares := make([]aop.Middleware, 0, 10)
	for i := 0; i < 10; i++ {
		middlewares = append(middlewares, aop.MiddlewareFunc(noopMiddleware))
	}
	handler := buildHTTPHandler(b, middlewares...)
	req := httptest.NewRequest(http.MethodGet, "http://bench.local/users/1", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

// BenchmarkHTTP_BindValidate 衡量 Body 解码 + validator 校验的成本。
func BenchmarkHTTP_BindValidate(b *testing.B) {
	handler := buildHTTPHandler(b)
	payload := []byte(`{"name":"alice","email":"alice@example.com"}`)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://bench.local/users", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

// ---------------------------------------------------------------------------
// DI 场景
// ---------------------------------------------------------------------------

type benchL1 struct{ V int }
type benchL2 struct{ One *benchL1 }
type benchL3 struct{ Two *benchL2 }
type benchL4 struct{ Three *benchL3 }
type benchL5 struct{ Four *benchL4 }

// BenchmarkDI_ResolveNested 衡量 5 层字段注入的解析成本（每次都是新实例）。
func BenchmarkDI_ResolveNested(b *testing.B) {
	container := di.NewContainer()
	container.RegisterClass(reflect.TypeFor[*benchL1](), reflect.TypeFor[benchL1](), di.Transient)
	container.RegisterClass(reflect.TypeFor[*benchL2](), reflect.TypeFor[benchL2](), di.Transient)
	container.RegisterClass(reflect.TypeFor[*benchL3](), reflect.TypeFor[benchL3](), di.Transient)
	container.RegisterClass(reflect.TypeFor[*benchL4](), reflect.TypeFor[benchL4](), di.Transient)
	container.RegisterClass(reflect.TypeFor[*benchL5](), reflect.TypeFor[benchL5](), di.Transient)
	container.SetInjectionAudit(false)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value, err := container.Get(reflect.TypeFor[*benchL5]())
		if err != nil {
			b.Fatalf("resolve: %v", err)
		}
		if value == nil {
			b.Fatalf("nil instance")
		}
	}
}
