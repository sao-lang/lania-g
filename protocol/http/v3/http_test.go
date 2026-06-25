package http

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpprotocol "github.com/sao-lang/lania-g/protocol/http/v3/protocol"
)

type fakeHost struct {
	rt  *runtime.Runtime
	reg *registry.Registry
}

type formCookieCtrl struct{}

func (c *formCookieCtrl) Upload(args struct {
	Name   httpbinding.Form[string]   `form:"name" binding:"required"`
	SID    httpbinding.Cookie[string] `cookie:"sid" binding:"required"`
	All    httpbinding.Cookies
	Avatar httpbinding.File `file:"avatar" required:"true"`
}) (any, error) {
	out := map[string]any{
		"name":   args.Name.Value,
		"sid":    args.SID.Value,
		"cookie": args.All["sid"],
		"avatar": "",
	}
	if args.Avatar.Value != nil {
		out["avatar"] = args.Avatar.Value.Filename
	}
	return out, nil
}

func TestAdapter_FormAndCookieBinding(t *testing.T) {
	a := New()
	rt := runtime.NewRuntime()
	reg := registry.New()
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	httpbinding.RegisterDefaults(rt)
	registerHandler(t, rt, http.MethodPost, "/u", &formCookieCtrl{}, "Upload")

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("name", "bob")
	fw, err := w.CreateFormFile("avatar", "a.txt")
	if err != nil {
		t.Fatalf("CreateFormFile avatar: %v", err)
	}
	_, _ = fw.Write([]byte("A"))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "http://example.com/u", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "sid", Value: "abc"})

	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"name":"bob"`) || !strings.Contains(body, `"sid":"abc"`) || !strings.Contains(body, `"avatar":"a.txt"`) {
		t.Fatalf("unexpected response body: %s", body)
	}
}

type validateCtrl struct{}

type validateDTO struct {
	Email string `json:"email" validate:"email"`
}

func (c *validateCtrl) H(args httpbinding.Bind[httpbinding.Body[validateDTO]]) (any, error) {
	return map[string]any{"ok": true}, nil
}

func TestAdapter_ValidatorV10_Email(t *testing.T) {
	a := New().WithValidatorV10()
	rt := runtime.NewRuntime()
	reg := registry.New()
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	httpbinding.RegisterDefaults(rt)
	registerHandler(t, rt, http.MethodPost, "/v", &validateCtrl{}, "H")

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v", strings.NewReader(`{"email":"not-an-email"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"kind":"Validation"`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func (h *fakeHost) Runtime() *runtime.Runtime    { return h.rt }
func (h *fakeHost) Registry() *registry.Registry { return h.reg }
func (h *fakeHost) ModuleRef() *module.ModuleRef { return nil }

var _ adapter.Host = (*fakeHost)(nil)

type testCtrl struct{}

func (c *testCtrl) Handle() (any, error) { return nil, nil }

func TestPluginScan_RequiresExplicitRegistry(t *testing.T) {
	ctrl := &testCtrl{}
	root := module.CreateModule(nil, nil, []any{ctrl}, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	api := NewAPI(reg, nil)
	api.Controller("/users", ctrl).Get("/ping", ctrl.Handle).Build()

	_, err := (&Plugin{}).Scan(moduleRef, nil)
	if err == nil {
		t.Fatalf("expected missing registry error")
	}
	if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile with explicit registry: %v", err)
	}
	if compiled == nil {
		t.Fatalf("expected compiled app")
	}
}

func mountForTest(t *testing.T, a *Adapter) {
	t.Helper()
	rt := runtime.NewRuntime()
	reg := registry.New()
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}

	// 确保已注册 HTTP binding resolver（文件上传相关测试依赖它）。
	httpbinding.RegisterDefaults(rt)

	// 注册一个最小 handler，确保 middleware 调用 `Next()` 时 `runtime.Execute` 可以正常工作。
	ctrl := &testCtrl{}
	h, err := runtime.NewHandler(ctrl, "Handle")
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	h.Meta.RouteKey = runtime.BuildRouteKey(httpprotocol.Protocol, http.MethodGet, "/test")
	h.Meta.Protocol = httpprotocol.Protocol
	h.Meta.StatusCode = 204
	rt.GetRouter().Register(h.Meta.RouteKey, h)
}

type uploadCtrl struct{}

func (c *uploadCtrl) Upload(args struct {
	Avatar httpbinding.File  `file:"avatar" required:"true"`
	Photos httpbinding.Files `files:"photos"`
}) (any, error) {
	out := map[string]any{
		"avatar": "",
		"count":  0,
	}
	if args.Avatar.Value != nil {
		out["avatar"] = args.Avatar.Value.Filename
	}
	if args.Photos.Value != nil {
		out["count"] = len(args.Photos.Value)
	}
	return out, nil
}

func registerHandler(t *testing.T, rt *runtime.Runtime, method, path string, instance any, methodName string) {
	t.Helper()
	h, err := runtime.NewHandler(instance, methodName)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	h.Meta.RouteKey = runtime.BuildRouteKey(httpprotocol.Protocol, method, path)
	h.Meta.Protocol = httpprotocol.Protocol
	rt.GetRouter().Register(h.Meta.RouteKey, h)
}

func TestAdapter_MountHandler_RespectsBasePath(t *testing.T) {
	a := New().WithBasePath("/api")
	mountForTest(t, a)

	a.MountHandler("/mounted", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.Path))
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/mounted/ok", nil)
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// 在挂载匹配和 handler 执行前会先剥离 `basePath`。
	if got := rec.Body.String(); got != "/mounted/ok" {
		t.Fatalf("body = %q, want %q", got, "/mounted/ok")
	}
}

func TestAdapter_CORS_Preflight(t *testing.T) {
	a := New().EnableCors(nil)
	mountForTest(t, a)

	req := httptest.NewRequest(http.MethodOptions, "http://example.com/any", nil)
	req.Header.Set("Origin", "https://foo.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "" {
		t.Fatalf("missing Access-Control-Allow-Origin")
	}
}

func TestAdapter_ReadBody_RestoresForDownstream(t *testing.T) {
	a := New()
	mountForTest(t, a)

	var got string
	a.WithNextHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
	}))

	// 调用两次 `Next()`，确保 `runtime.Execute` 之后还能继续进入下游 `nextHandler`。
	a.UseAfter(func(ctx *httpbinding.HttpContext) error {
		if err := ctx.Next(); err != nil {
			return err
		}
		return ctx.Next()
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", strings.NewReader("hello"))

	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if got != "hello" {
		t.Fatalf("downstream body = %q, want %q", got, "hello")
	}
}

func TestAdapter_FileFiles_BindInCompositeArgs(t *testing.T) {
	a := New()
	// 自定义挂载，同时注册一个上传路由 handler。
	rt := runtime.NewRuntime()
	reg := registry.New()
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	httpbinding.RegisterDefaults(rt)
	registerHandler(t, rt, http.MethodPost, "/upload", &uploadCtrl{}, "Upload")

	var body strings.Builder
	w := multipart.NewWriter(&body)

	// 单文件字段：avatar
	fw, err := w.CreateFormFile("avatar", "a.txt")
	if err != nil {
		t.Fatalf("CreateFormFile avatar: %v", err)
	}
	_, _ = fw.Write([]byte("A"))

	// 多文件字段：photos
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="photos"; filename="p1.txt"`)
	hdr.Set("Content-Type", "text/plain")
	pw1, err := w.CreatePart(hdr)
	if err != nil {
		t.Fatalf("CreatePart photos1: %v", err)
	}
	_, _ = pw1.Write([]byte("P1"))

	hdr2 := make(textproto.MIMEHeader)
	hdr2.Set("Content-Disposition", `form-data; name="photos"; filename="p2.txt"`)
	hdr2.Set("Content-Type", "text/plain")
	pw2, err := w.CreatePart(hdr2)
	if err != nil {
		t.Fatalf("CreatePart photos2: %v", err)
	}
	_, _ = pw2.Write([]byte("P2"))

	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "http://example.com/upload", strings.NewReader(body.String()))
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"avatar":"a.txt"`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"count":2`) {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

type directWriteCtrl struct {
	filePath string
}

func (c *directWriteCtrl) DoRedirect(ctx httpbinding.Context) (any, error) {
	_ = ctx.Redirect(http.StatusFound, "/x")
	return nil, nil
}

func (c *directWriteCtrl) DoServeFile(ctx httpbinding.Context) (any, error) {
	_ = ctx.ServeFile(c.filePath)
	return nil, nil
}

func TestAdapter_ContextRedirect_SkipsNormalResponseWrite(t *testing.T) {
	a := New()
	rt := runtime.NewRuntime()
	reg := registry.New()
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	httpbinding.RegisterDefaults(rt)
	registerHandler(t, rt, http.MethodGet, "/redir", &directWriteCtrl{}, "DoRedirect")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/redir", nil)
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/x" {
		t.Fatalf("Location = %q, want %q", loc, "/x")
	}
}

func TestAdapter_ContextServeFile_SkipsNormalResponseWrite(t *testing.T) {
	tmp, err := os.CreateTemp("", "gokernel_v3_http_*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, _ = tmp.WriteString("hello-file")
	_ = tmp.Close()

	a := New()
	rt := runtime.NewRuntime()
	reg := registry.New()
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	httpbinding.RegisterDefaults(rt)
	registerHandler(t, rt, http.MethodGet, "/file", &directWriteCtrl{filePath: tmp.Name()}, "DoServeFile")

	req := httptest.NewRequest(http.MethodGet, "http://example.com/file", nil)
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "hello-file" {
		t.Fatalf("body = %q, want %q", got, "hello-file")
	}
}
