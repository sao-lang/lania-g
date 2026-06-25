package aop

// CanActivate 定义守卫的最小行为：判断当前请求是否允许继续执行。
type CanActivate interface {
	CanActivate(ctx *ExecutionContext) (bool, error)
}

// Guard 是框架对“守卫”对象的统一抽象。
type Guard interface {
	CanActivate
}

// GuardConstructor 用于延迟创建 Guard 实例。
type GuardConstructor func() Guard

// GuardFunc 是 Guard 的函数式写法。
type GuardFunc func(ctx *ExecutionContext) (bool, error)

// CanActivate 让 GuardFunc 适配 Guard 接口。
//
// 约定：
// - 返回 (true, nil)：允许继续执行
// - 返回 (false, nil)：拒绝执行（上层通常会转换为框架的 GuardRejected 类错误）
// - 返回 (_, err)：发生错误并中断执行
func (f GuardFunc) CanActivate(ctx *ExecutionContext) (bool, error) {
	return f(ctx)
}

// WrapGuard 将 Guard（对象形式）包装为 GuardFunc（函数形式）。
//
// 这样上层 pipeline 可以统一按函数切片执行 guards，减少接口调用与分配。
func WrapGuard(guard Guard) GuardFunc {
	return func(ctx *ExecutionContext) (bool, error) {
		return guard.CanActivate(ctx)
	}
}
