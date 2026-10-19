// mount.go 把 Swagger UI 与 OpenAPI 文档挂载到 HTTP 适配器。
package swagger

import (
	"net/http"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
)

// MountHTTP 实现 adapter.HTTPMounterProvider。
//
// 它在应用声明阶段之后执行：此时 HTTP 路由已经收集完毕，
// 因此可以一次性生成 OpenAPI 并挂载 UI/spec 端点。
func (m *integrationModule) MountHTTP(req adapter.HTTPMountRequest) error {
	if m == nil || m.builder == nil || req.Mounter == nil {
		return nil
	}
	for _, name := range m.config.BearerAuth {
		m.builder.AddBearerAuth(name)
	}
	for _, server := range m.config.Servers {
		m.builder.AddServer(server.URL, server.Description)
	}
	for _, tag := range m.config.Tags {
		m.builder.AddTag(tag.Name, tag.Description)
	}
	if m.config.DefaultErrorResponse != "" {
		m.builder.SetDefaultErrorResponse(m.config.DefaultErrorResponse, nil,
			http.StatusBadRequest, http.StatusInternalServerError)
	}
	if req.Host != nil {
		buildFromHTTPRegistry(m.builder, req.Host.Registry())
	}

	ui := m.uiConfig
	if ui == nil {
		ui = DefaultUIConfig()
	}

	specURL := ui.SpecURL
	if specURL == "" {
		specURL = "/swagger.json"
	}
	req.Mounter.Handle(specURL, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := m.builder.ToJSON()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))

	swaggerURL := ui.SwaggerURL
	if swaggerURL == "" {
		swaggerURL = "/swagger"
	}
	req.Mounter.Handle(swaggerURL, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(GenerateSwaggerUIHTML(ui)))
	}))

	redocURL := ui.RedocURL
	if redocURL == "" {
		redocURL = "/redoc"
	}
	req.Mounter.Handle(redocURL, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(GenerateRedocHTML(ui)))
	}))
	return nil
}

var _ adapter.HTTPMounterProvider = (*integrationModule)(nil)
