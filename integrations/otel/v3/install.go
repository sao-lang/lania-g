// install.go 实现 otel 集成在应用启动期的安装与接线逻辑。
package otel

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
)

// ApplicationInstaller 描述一个可安装全局 middleware/interceptor 的对象（通常是 application/app）。
type ApplicationInstaller interface {
	UseGlobalMiddlewares(...aop.MiddlewareFunc)
	UseGlobalInterceptors(...aop.InterceptorFunc)
}

// Install 将 OTel middleware/interceptor 安装为全局 AOP。
func Install(into ApplicationInstaller, telemetry *Telemetry) {
	if into == nil || telemetry == nil {
		return
	}
	into.UseGlobalMiddlewares(Middleware(telemetry))
	into.UseGlobalInterceptors(Interceptor(telemetry))
}

// InstallOnFactory 尝试通过反射把 OTel middleware/interceptor 安装到 factory 上。
//
// 适用于无法直接依赖 ApplicationInstaller 接口的场景。
func InstallOnFactory(into any, telemetry *Telemetry) error {
	if into == nil || telemetry == nil {
		return nil
	}
	value := reflect.ValueOf(into)
	middlewareMethod := value.MethodByName("UseGlobalMiddleware")
	interceptorMethod := value.MethodByName("UseGlobalInterceptors")
	if !middlewareMethod.IsValid() || !interceptorMethod.IsValid() {
		return fmt.Errorf("factory does not expose global otel install hooks")
	}
	middlewareMethod.Call([]reflect.Value{reflect.ValueOf(Middleware(telemetry))})
	interceptorMethod.Call([]reflect.Value{reflect.ValueOf(Interceptor(telemetry))})
	return nil
}
