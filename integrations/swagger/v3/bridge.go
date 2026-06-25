// bridge.go 实现 swagger 集成与框架其余协议/组件之间的桥接能力。
package swagger

import (
	"net/http"

	"github.com/sao-lang/lania-g/application/v3"
	corehttp "github.com/sao-lang/lania-g/protocol/http/v3"
	coremodule "github.com/sao-lang/lania-g/kernel/v3/module"
)

// ServeHTTPBridge 将 swagger UI 与 spec 通过 HTTP Adapter 挂载。
// 需要在 Application 装好 HTTP Adapter 后调用。
func ServeHTTPBridge(app *application.Application) {
	apiAny, ok := app.API(corehttp.AdapterID)
	if !ok || apiAny == nil {
		return
	}
	api, ok := apiAny.(*corehttp.API)
	if !ok {
		return
	}
	// 从根容器中解析 builder 和 UI 配置
	builder, err := coremodule.GetByType[*Builder](app.ModuleRef())
	if err != nil {
		return
	}
	ui, _ := coremodule.GetByType[*UIConfig](app.ModuleRef())
	if ui == nil {
		ui = DefaultUIConfig()
	}
	specURL := ui.SpecURL
	if specURL == "" {
		specURL = "/swagger.json"
	}
	api.MountHandler(specURL, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := builder.ToJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	uiURL := ui.SwaggerURL
	if uiURL == "" {
		uiURL = "/swagger"
	}
	api.MountHandler(uiURL, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := GenerateSwaggerUIHTML(ui)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	}))
	redocURL := ui.RedocURL
	if redocURL == "" {
		redocURL = "/redoc"
	}
	api.MountHandler(redocURL, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := GenerateRedocHTML(ui)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	}))
}
