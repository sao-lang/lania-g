// gateway_refs.go 提供 WS gateway 相关引用与辅助类型。
package ws

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	corescanner "github.com/sao-lang/lania-g/kernel/v3/scanner"
)

type gatewayRef struct {
	namespace    string
	gatewayToken reflect.Type
	container    *di.Container
}

// resolve 从该 gatewayRef 绑定的 DI container 中解析出 gateway 实例。
//
// 语义：
// - gatewayToken 是 receiver 的类型 token（统一为指针类型 *T）
// - container 是 gateway 所属模块的容器（不是 request-scope child）
//
// 注意：解析失败会返回 nil（WS adapter 的 hook 调用方会跳过 nil gateway）。
func (r gatewayRef) resolve() any {
	if r.container == nil || r.gatewayToken == nil {
		return nil
	}
	inst, err := r.container.Get(r.gatewayToken)
	if err != nil {
		return nil
	}
	return inst
}

// buildGatewayRefs 根据 registry 中声明的 handlers，构建 namespace -> gatewayRef 列表。
//
// 为什么需要它：
// - WS adapter 在注册 namespace 的 OnConnect/OnDisconnect/OnError hooks 时，需要知道“该 namespace 下有哪些 gateway”
// - gateway 可能作为 controller/resolver 出现在多个模块中，因此需要结合 moduleRef 的模块树推导其 owner 模块容器
//
// 推导逻辑（简化）：
// 1) 通过 core/scanner.BuildSnapshot(moduleRef) 获取 controller/resolver 的 owner 列表
// 2) ownerByType：按 gateway 的指针类型 token 汇总候选 owners
// 3) 若同一个 token 只有一个 owner，则再用实例指针建立 ownerByPtr，用于后续消歧
// 4) 遍历 decls 中的 HandlerDefinition：
//   - 规范化 namespace（Prefix）
//   - 规范化 gateway token（指针类型）
//   - 选择唯一 owner（无 owner/多 owner 且无法消歧都会报错）
//   - 去重（同一个 namespace 下，同一 token 只保留一次）
func buildGatewayRefs(moduleRef *module.ModuleRef, decls []any) (map[string][]gatewayRef, error) {
	if moduleRef == nil {
		return nil, &kerrors.KernelError{Kind: kerrors.KindDI, Message: "moduleRef is nil", Meta: map[string]any{"stage": "ws_gateway_refs"}}
	}

	snapshot := corescanner.BuildSnapshot(moduleRef)
	ownerByType := make(map[reflect.Type][]corescanner.ModuleOwner)
	for t, owners := range snapshot.ControllerOwners {
		ownerByType[t] = append(ownerByType[t], owners...)
	}
	for t, owners := range snapshot.ResolverOwners {
		ownerByType[t] = append(ownerByType[t], owners...)
	}

	// 当某个 token 只有唯一 owner 时，再额外按实例指针建索引。
	// 这样一旦后面遇到“同类型多实例，但当前实例指针可识别”的场景，就能继续精确消歧。
	ownerByPtr := make(map[uintptr]corescanner.ModuleOwner)
	for _, gw := range append(append([]any{}, snapshot.Controllers...), snapshot.Resolvers...) {
		token := normalizeGatewayToken(gw)
		if token == nil {
			continue
		}
		if owners := ownerByType[token]; len(owners) == 1 {
			if ptr := instancePointer(gw); ptr != 0 {
				ownerByPtr[ptr] = owners[0]
			}
		}
	}

	refs := make(map[string][]gatewayRef)
	seen := make(map[string]map[reflect.Type]bool)

	for _, item := range decls {
		def, ok := item.(*HandlerDefinition)
		if !ok || def == nil || def.Gateway == nil {
			continue
		}
		ns := normalizeNamespace(def.Prefix)
		token := normalizeGatewayToken(def.Gateway)
		if token == nil {
			return nil, fmt.Errorf("ws gateway token is nil")
		}

		candidates := ownerByType[token]
		var own corescanner.ModuleOwner
		switch len(candidates) {
		case 0:
			return nil, fmt.Errorf("ws module owner not found for receiver token %s", token.String())
		case 1:
			own = candidates[0]
		default:
			// 多 owner 时优先尝试按实例指针消歧；如果仍不唯一，就要求用户修模块归属。
			if ptr := instancePointer(def.Gateway); ptr != 0 {
				if byPtr, ok := ownerByPtr[ptr]; ok {
					own = byPtr
					break
				}
			}
			return nil, fmt.Errorf("ws module owner ambiguous for receiver token %s", token.String())
		}

		if seen[ns] == nil {
			seen[ns] = make(map[reflect.Type]bool)
		}
		if seen[ns][token] {
			// 同一 namespace 下，同一个 gateway token 只需要为 hook 索引保留一次。
			continue
		}
		seen[ns][token] = true

		refs[ns] = append(refs[ns], gatewayRef{namespace: ns, gatewayToken: token, container: own.Container})
	}

	return refs, nil
}
