// scanner.go 提供从 `ModuleRef` 提炼“编译器视角模块快照”的能力。
//
// 它不参与运行时执行，也不直接做协议编译；
// 它的职责更像是把模块树转换成一份 compiler/plugin 易消费的只读视图。
package scanner

import (
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// ModuleOwner 表示某个 controller / resolver / module 在扫描结果中的归属信息。
//
// compiler 会利用这份信息反推出：
// - 当前声明属于哪个模块
// - 编译和运行时应该使用哪个 DI container
type ModuleOwner struct {
	ModuleKey  string
	ModuleType reflect.Type
	Container  *di.Container
}

// Snapshot 是 compiler/插件扫描阶段需要的“模块视图”：
// - 汇总所有 module/providers/controllers/resolvers
// - 记录 controller/resolver 的“归属模块”，用于推导 ModuleKey 与选择 DI container
type Snapshot struct {
	Modules          []module.Module
	Providers        map[interface{}]*di.Provider
	Controllers      []interface{}
	Resolvers        []interface{}
	ModuleOwners     map[reflect.Type]ModuleOwner
	ControllerOwners map[reflect.Type][]ModuleOwner
	ResolverOwners   map[reflect.Type][]ModuleOwner
}

// BuildSnapshot 从 ModuleRef 构建扫描快照。
//
// 它会把模块树中的 providers、controllers、resolvers 和 owner 信息汇总到同一个结构，
// 方便 compiler 与各协议插件在后续编译阶段统一消费。
//
// 注意：token 会统一规范化为“指针类型”，保证 container.Get(token) 的一致性。
func BuildSnapshot(moduleRef *module.ModuleRef) *Snapshot {
	if moduleRef == nil {
		return &Snapshot{
			Providers:        make(map[interface{}]*di.Provider),
			ModuleOwners:     make(map[reflect.Type]ModuleOwner),
			ControllerOwners: make(map[reflect.Type][]ModuleOwner),
			ResolverOwners:   make(map[reflect.Type][]ModuleOwner),
		}
	}

	s := &Snapshot{
		Providers:        make(map[interface{}]*di.Provider),
		ModuleOwners:     make(map[reflect.Type]ModuleOwner),
		ControllerOwners: make(map[reflect.Type][]ModuleOwner),
		ResolverOwners:   make(map[reflect.Type][]ModuleOwner),
	}

	for moduleType, mod := range moduleRef.GetAllModulesWithTypes() {
		if mod == nil || mod.Metadata() == nil {
			continue
		}
		meta := mod.Metadata()
		owner := ModuleOwner{
			ModuleKey:  moduleType.String(),
			ModuleType: moduleType,
			Container:  mod.Container(),
		}
		s.Modules = append(s.Modules, mod)
		s.ModuleOwners[moduleType] = owner
		for _, provider := range meta.Providers {
			if provider != nil {
				// provider 这里按 token 聚合，后续 compiler 更关心“某个 token 是否存在”。
				s.Providers[provider.Token] = provider
			}
		}
		for _, controller := range meta.Controllers {
			if controller == nil {
				continue
			}
			s.Controllers = append(s.Controllers, controller)
			token := normalizeToken(controller)
			if token != nil {
				s.ControllerOwners[token] = append(s.ControllerOwners[token], owner)
			}
		}
		for _, resolver := range meta.Resolvers {
			if resolver == nil {
				continue
			}
			s.Resolvers = append(s.Resolvers, resolver)
			token := normalizeToken(resolver)
			if token != nil {
				s.ResolverOwners[token] = append(s.ResolverOwners[token], owner)
			}
		}
	}

	return s
}

// normalizeToken 将 controller/resolver 的类型规范化为“指针类型 token”。
//
// 统一用指针类型的原因：
// - container 中 controller/resolver 通常按 *T 注册（见 module.BaseModule.Init）
// - 这样 compiler/runtime 在按类型解析 receiver 时不会因值类型/指针类型差异导致 miss
func normalizeToken(v interface{}) reflect.Type {
	if v == nil {
		return nil
	}
	t := reflect.TypeOf(v)
	if t.Kind() != reflect.Ptr {
		t = reflect.PointerTo(t)
	}
	return t
}
