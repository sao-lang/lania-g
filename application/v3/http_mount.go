// http_mount.go 提供 Application 作为共享 HTTP 挂载宿主时的桥接实现。
//
// 这个能力主要给像 scheduler bridge、静态资源、健康检查这类“直接挂一个原生 http.Handler”
// 的场景使用，而无需再走一遍 HTTP adapter 的 DSL。
package application

import (
	"fmt"
	"net/http"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
)

// MountHTTP 实现 `adapter.HTTPMountHost`。
// 它本质上是把“应用级挂载请求”转发给已经挂载的 HTTP adapter API。
func (a *Application) MountHTTP(pattern string, handler http.Handler) error {
	if a == nil {
		return fmt.Errorf("application is nil")
	}
	if pattern == "" || handler == nil {
		return fmt.Errorf("invalid mount: pattern or handler is nil")
	}

	api, ok := a.API("http")
	if !ok || api == nil {
		return fmt.Errorf("no mounted http adapter")
	}
	mounter, ok := api.(interface{ MountHandler(string, http.Handler) })
	if !ok {
		return fmt.Errorf("mounted http adapter api does not support MountHandler")
	}
	mounter.MountHandler(pattern, handler)
	return nil
}

var _ adapter.HTTPMountHost = (*Application)(nil)
