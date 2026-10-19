// failed.go 提供“装配期已失败”的模块占位实现。
//
// 集成模块的构造函数需要保持 `X.Module(cfg) module.Module` 这一单入口签名，
// 但配置/客户端创建可能失败。把错误暂存在模块里，
// 让它在 Init 阶段返回，从而把错误出口统一收敛到 application.New。
package module

// failedModule 是一个 Init 阶段必定返回指定错误的模块。
type failedModule struct {
	*BaseModule
	err error
}

// Failed 返回一个在 Init 阶段返回 err 的模块。
func Failed(err error) Module {
	return &failedModule{BaseModule: New(Metadata{}), err: err}
}

// Init 返回构造期保存的错误。
func (m *failedModule) Init() error {
	if m.err != nil {
		return m.err
	}
	return m.BaseModule.Init()
}
