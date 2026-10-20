// listen.go 实现应用级的共享监听地址设置。
//
// 多协议同进程时，HTTP 是"共享监听地址"的承载者：WS / GraphQL 在未显式
// 配置 Addr 时会挂到 HTTP adapter 上。`Listen` 让应用可以在 Run 之前统一
// 决定这个地址，而不必把端口写死在 HTTP 模块配置里。
package application

import (
	"fmt"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
)

// Listen 为共享监听类 adapter 统一设置监听地址。
//
// 约束：
// - 必须在 Run / Start 之前调用
// - 最多只能有一个 adapter 要求共享监听（通常是 HTTP）
// - 调用后仍可通过 Run 启动；不需要为了生效而改用其他启动方式
func (a *Application) Listen(addr string) error {
	if a == nil {
		return fmt.Errorf("application is nil")
	}

	var target adapter.SharedListenerConfigurator
	for _, adp := range a.adapterList {
		configurator, ok := adp.(adapter.SharedListenerConfigurator)
		if !ok || !configurator.RequiresSharedListen() {
			continue
		}
		if target != nil {
			return fmt.Errorf("multiple adapters require shared listener address: %s", adp.ID())
		}
		target = configurator
	}
	if target == nil {
		return fmt.Errorf("no mounted adapter requires a shared listener address")
	}
	return target.ConfigureSharedListen(addr)
}
