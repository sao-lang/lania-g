// http_mount.go 让 Application 成为模块挂载原生 HTTP handler 的宿主。
package application

import (
	"fmt"
	"net/http"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
)

// httpMountAdapter 是“支持挂载原生 handler”的适配器能力集。
type httpMountAdapter interface {
	MountHandler(pattern string, handler http.Handler)
	ServeStatic(prefix, root string)
}

// Handle 把原生 http.Handler 挂到给定前缀。
func (a *Application) Handle(pattern string, handler http.Handler) {
	_ = a.MountHTTP(pattern, handler)
}

// Static 把本地目录作为静态资源挂到给定前缀。
func (a *Application) Static(prefix, root string) {
	if mounter, ok := a.httpMounter(); ok {
		mounter.ServeStatic(prefix, root)
	}
}

// MountHTTP 实现 adapter.HTTPMountHost 与 adapter.Mounter。
func (a *Application) MountHTTP(pattern string, handler http.Handler) error {
	if a == nil {
		return fmt.Errorf("application is nil")
	}
	if pattern == "" || handler == nil {
		return fmt.Errorf("invalid mount: pattern or handler is nil")
	}
	mounter, ok := a.httpMounter()
	if !ok {
		return fmt.Errorf("no mounted http adapter")
	}
	mounter.MountHandler(pattern, handler)
	return nil
}

func (a *Application) httpMounter() (httpMountAdapter, bool) {
	if a == nil {
		return nil, false
	}
	for _, adp := range a.adapterList {
		if mounter, ok := adp.(httpMountAdapter); ok {
			return mounter, true
		}
	}
	return nil, false
}

var (
	_ adapter.HTTPMountHost = (*Application)(nil)
	_ adapter.Mounter       = (*Application)(nil)
)
