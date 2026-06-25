// bridge.go 实现 terminus 集成与框架其余协议/组件之间的桥接能力。
package terminus

import (
	"encoding/json"
	"net/http"

	"github.com/sao-lang/lania-g/application/v3"
	corehttp "github.com/sao-lang/lania-g/protocol/http/v3"
	coremodule "github.com/sao-lang/lania-g/kernel/v3/module"
)

// ServeHTTPBridge 把健康检查端点挂载到 HTTP adapter。
func ServeHTTPBridge(app *application.Application, path string) {
	if path == "" {
		path = "/health"
	}
	apiAny, ok := app.API(corehttp.AdapterID)
	if !ok || apiAny == nil {
		return
	}
	api, ok := apiAny.(*corehttp.API)
	if !ok {
		return
	}
	service, err := coremodule.GetByType[*HealthService](app.ModuleRef())
	if err != nil {
		return
	}
	api.MountHandler(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := service.Check()
		status := http.StatusOK
		if result.Status == StatusFail {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(result)
	}))
}
