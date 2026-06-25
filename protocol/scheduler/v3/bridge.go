// bridge.go 提供 Scheduler adapter 与应用生命周期之间的桥接逻辑。
package scheduler

import (
	"encoding/json"
	"net/http"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
)

// SnapshotHandler 返回一个 HTTP handler，用于输出 scheduler 运行期快照。
// 这是一个只读观测入口，不参与 job 调度本身。
func SnapshotHandler(adp *Adapter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if adp == nil {
			http.Error(w, "scheduler adapter is nil", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(adp.Snapshot())
	})
}

// MountHTTPBridge 把 scheduler 快照 handler 挂载到 HTTP adapter 上。
// 这样应用无需自己再写一层 controller，就能把调度状态暴露到某个 HTTP 路径。
func MountHTTPBridge(host adapter.HTTPMountHost, adp *Adapter, pattern string) error {
	if pattern == "" {
		pattern = "/scheduler"
	}
	return host.MountHTTP(pattern, SnapshotHandler(adp))
}
