// declare.go 定义“声明阶段”的协议扩展点。
//
// 启动顺序固定为：
//
//	模块树加载 → Init → Declare（协议声明收集）→ Compile → Install → Run
//
// Declarer 由协议适配器实现：它在模块全部 Init 完成后被调用一次，
// 按模块树顺序遍历控制器，把声明写入所属模块的声明仓库。
// 这样“接口归属于哪个模块”在声明阶段就确定，不再依赖运行期推断。
package module

import (
	"fmt"
)

// Declarer 由协议适配器实现。
type Declarer interface {
	// Declare 在模块树完成 Init 后被调用。
	//
	// 约定：
	//   - 必须按模块树深度优先顺序遍历
	//   - 只能读取容器中的实例，不得注册新的 provider
	//   - 返回 error 表示声明失败，应用启动应立即中止
	Declare(root *ModuleRef) error
}

// WalkModules 按深度优先顺序遍历模块树。
func WalkModules(root Module, fn func(Module) error) error {
	if root == nil || fn == nil {
		return nil
	}
	seen := make(map[Module]bool)
	return walkModule(root, seen, fn)
}

func walkModule(mod Module, seen map[Module]bool, fn func(Module) error) error {
	if mod == nil || seen[mod] {
		return nil
	}
	seen[mod] = true
	if err := fn(mod); err != nil {
		return err
	}
	metadata := mod.Metadata()
	if metadata == nil {
		return nil
	}
	for _, imp := range metadata.Imports {
		if err := walkModule(imp, seen, fn); err != nil {
			return err
		}
	}
	return nil
}

// ResolveController 从模块容器解析控制器实例。
//
// 控制器在 BaseModule.Init 阶段已注册进容器，因此这里不会触发构造。
func ResolveController(mod Module, controller any) (any, error) {
	if mod == nil || controller == nil {
		return controller, nil
	}
	container := mod.Container()
	if container == nil {
		return controller, nil
	}
	instance, err := container.Get(pointerType(controller))
	if err != nil {
		return nil, fmt.Errorf("resolve controller %T in module %T: %w", controller, mod, err)
	}
	return instance, nil
}

// DeclareAll 依次调用所有协议 declarer。
func DeclareAll(root *ModuleRef, declarers ...Declarer) error {
	if root == nil {
		return nil
	}
	for _, declarer := range declarers {
		if declarer == nil {
			continue
		}
		if err := declarer.Declare(root); err != nil {
			return err
		}
	}
	return nil
}
