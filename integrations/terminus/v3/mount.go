// mount.go 把健康检查端点挂载到 HTTP 适配器。
package terminus

import (
	"encoding/json"
	"net/http"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
)

// MountHTTP 实现 adapter.HTTPMounterProvider。
func (m *integrationModule) MountHTTP(req adapter.HTTPMountRequest) error {
	if m == nil || m.service == nil || req.Mounter == nil {
		return nil
	}
	path := m.config.Path
	if path == "" {
		path = "/health"
	}
	service := m.service
	req.Mounter.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := service.Check()
		status := http.StatusOK
		if result.Status == StatusFail {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(result)
	}))
	return nil
}

var _ adapter.HTTPMounterProvider = (*integrationModule)(nil)
