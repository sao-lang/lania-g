// bridge.go 实现 events 集成与框架其余协议/组件之间的桥接能力。
package events

import (
	stdctx "context"
	"fmt"
	"reflect"
	goruntime "runtime"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

const registryKindHandlers = "handlers"

// HandlerDefinition 描述一条事件与处理方法之间的声明式绑定。
type HandlerDefinition struct {
	Event       string
	Once        bool
	Receiver    any
	HandlerName string
}

// RegisterOn 注册一个常驻事件处理器声明。
func RegisterOn(reg *registry.Registry, event string, receiver any, handler any) *HandlerDefinition {
	if reg == nil {
		return RegisterOnCompat(event, receiver, handler)
	}
	return registerHandler(reg, event, false, receiver, handler)
}

// RegisterOnce 注册一个只执行一次的事件处理器声明。
func RegisterOnce(reg *registry.Registry, event string, receiver any, handler any) *HandlerDefinition {
	if reg == nil {
		return RegisterOnceCompat(event, receiver, handler)
	}
	return registerHandler(reg, event, true, receiver, handler)
}

// RegisterHandlers 批量注册事件处理器声明。
func RegisterHandlers(reg *registry.Registry, defs ...*HandlerDefinition) {
	if reg == nil {
		RegisterHandlersCompat(defs...)
		return
	}
	registerHandlers(reg, defs...)
}

// RegisterOnCompat 显式保留给迁移场景的全局事件声明入口，不作为新代码默认写法。
func RegisterOnCompat(event string, receiver any, handler any) *HandlerDefinition {
	return registerHandler(registry.GlobalWithUsage("integrations/events.RegisterOnCompat"), event, false, receiver, handler)
}

// RegisterOnceCompat 显式保留给迁移场景的全局事件声明入口，不作为新代码默认写法。
func RegisterOnceCompat(event string, receiver any, handler any) *HandlerDefinition {
	return registerHandler(registry.GlobalWithUsage("integrations/events.RegisterOnceCompat"), event, true, receiver, handler)
}

// RegisterHandlersCompat 显式保留给迁移场景的全局事件声明入口，不作为新代码默认写法。
func RegisterHandlersCompat(defs ...*HandlerDefinition) {
	registerHandlers(registry.GlobalWithUsage("integrations/events.RegisterHandlersCompat"), defs...)
}

func registerHandlers(reg *registry.Registry, defs ...*HandlerDefinition) {
	items := make([]any, 0, len(defs))
	for _, def := range defs {
		if def != nil {
			items = append(items, def)
		}
	}
	reg.RegisterDecl("events", registryKindHandlers, items...)
}

// AttachRegisteredHandlers 把 registry 中声明的处理器挂载到事件总线上。
// 推荐：显式传入应用实例使用的 registry。
func AttachRegisteredHandlers(bus *Bus, moduleRef *module.ModuleRef, reg *registry.Registry) error {
	if reg == nil {
		return fmt.Errorf("events bridge requires an explicit registry; pass the application registry instance or use AttachRegisteredHandlersCompat(...) only for migration-oriented global registry paths")
	}
	return attachRegisteredHandlers(bus, moduleRef, reg)
}

// AttachRegisteredHandlersCompat 从全局 registry 读取已声明的事件处理器，并挂载到事件总线上。
// 该入口保留给迁移场景，不作为实例级 application 主路径的默认写法。
func AttachRegisteredHandlersCompat(bus *Bus, moduleRef *module.ModuleRef) error {
	return attachRegisteredHandlers(bus, moduleRef, registry.Global())
}

func attachRegisteredHandlers(bus *Bus, moduleRef *module.ModuleRef, reg *registry.Registry) error {
	if bus == nil {
		return fmt.Errorf("events bridge requires bus")
	}
	for _, item := range reg.ListDecl("events", registryKindHandlers) {
		def, ok := item.(*HandlerDefinition)
		if !ok || def == nil {
			continue
		}
		receiver, err := resolveReceiver(moduleRef, def.Receiver)
		if err != nil {
			return err
		}
		handler, err := buildHandler(receiver, def.HandlerName)
		if err != nil {
			return err
		}
		if def.Once {
			bus.Once(def.Event, handler)
		} else {
			bus.On(def.Event, handler)
		}
	}
	return nil
}

func registerHandler(reg *registry.Registry, event string, once bool, receiver any, handler any) *HandlerDefinition {
	def := &HandlerDefinition{
		Event:       event,
		Once:        once,
		Receiver:    receiver,
		HandlerName: findMethodName(receiver, handler),
	}
	reg.RegisterDecl("events", registryKindHandlers, def)
	return def
}

func resolveReceiver(moduleRef *module.ModuleRef, receiver any) (any, error) {
	if receiver == nil {
		return nil, fmt.Errorf("events receiver is nil")
	}
	if moduleRef == nil {
		return receiver, nil
	}
	token := reflect.TypeOf(receiver)
	if token.Kind() != reflect.Ptr {
		token = reflect.PointerTo(token)
	}
	resolved, err := moduleRef.Get(token)
	if err != nil {
		return receiver, nil
	}
	return resolved, nil
}

func buildHandler(receiver any, methodName string) (Handler, error) {
	if receiver == nil || methodName == "" {
		return nil, fmt.Errorf("invalid events handler registration")
	}
	method := reflect.ValueOf(receiver).MethodByName(methodName)
	if !method.IsValid() {
		return nil, fmt.Errorf("events method not found: %s", methodName)
	}
	return func(ctx stdctx.Context, args ...interface{}) error {
		in := make([]reflect.Value, 0, method.Type().NumIn())
		argIndex := 0
		for i := 0; i < method.Type().NumIn(); i++ {
			paramType := method.Type().In(i)
			if paramType == reflect.TypeFor[stdctx.Context]() {
				if ctx == nil {
					ctx = stdctx.Background()
				}
				in = append(in, reflect.ValueOf(ctx))
				continue
			}
			if argIndex >= len(args) {
				in = append(in, reflect.Zero(paramType))
				continue
			}
			value := reflect.ValueOf(args[argIndex])
			argIndex++
			if value.IsValid() && value.Type().AssignableTo(paramType) {
				in = append(in, value)
				continue
			}
			if value.IsValid() && value.Type().ConvertibleTo(paramType) {
				in = append(in, value.Convert(paramType))
				continue
			}
			in = append(in, reflect.Zero(paramType))
		}
		out := method.Call(in)
		if len(out) > 0 {
			last := out[len(out)-1]
			if last.IsValid() && last.Type().Implements(reflect.TypeFor[error]()) && !last.IsNil() {
				return last.Interface().(error)
			}
		}
		return nil
	}, nil
}

func findMethodName(receiver any, handler any) string {
	rv := reflect.ValueOf(receiver)
	if !rv.IsValid() {
		return ""
	}
	rt := rv.Type()
	hv := reflect.ValueOf(handler)
	if !hv.IsValid() || hv.Kind() != reflect.Func {
		return ""
	}
	for i := 0; i < rt.NumMethod(); i++ {
		if rv.Method(i).Pointer() == hv.Pointer() {
			return rt.Method(i).Name
		}
	}
	if fn := goruntime.FuncForPC(hv.Pointer()); fn != nil {
		name := fn.Name()
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		name = strings.TrimSuffix(name, "-fm")
		for i := 0; i < rt.NumMethod(); i++ {
			if rt.Method(i).Name == name {
				return name
			}
		}
	}
	return ""
}

type lifecycleHook struct {
	bus       *Bus
	reg       *registry.Registry
	moduleRef *module.ModuleRef
}

// NewLifecycleHook 创建一个在应用启动时自动挂载事件处理器的生命周期钩子。
func NewLifecycleHook(bus *Bus, reg *registry.Registry) interface{ OnApplicationBootstrap() error } {
	return &lifecycleHook{bus: bus, reg: reg}
}

// NewLifecycleHookCompat 创建一个启动时从全局 registry 挂载事件处理器的兼容钩子。
func NewLifecycleHookCompat(bus *Bus) interface{ OnApplicationBootstrap() error } {
	return &lifecycleHook{bus: bus, reg: registry.Global()}
}

// OnApplicationBootstrap 在应用启动阶段完成已注册事件处理器的挂载。
func (h *lifecycleHook) OnApplicationBootstrap() error {
	return AttachRegisteredHandlers(h.bus, h.moduleRef, h.reg)
}

// SetRegistry 注入 registry，供启动阶段读取事件处理器声明。
func (h *lifecycleHook) SetRegistry(reg *registry.Registry) {
	h.reg = reg
}

// SetModuleRef 注入 ModuleRef，供启动阶段解析 receiver 实例。
func (h *lifecycleHook) SetModuleRef(ref *module.ModuleRef) {
	h.moduleRef = ref
}
