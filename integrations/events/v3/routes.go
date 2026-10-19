// routes.go 定义事件订阅声明。
//
// 声明方式与 HTTP 一致：控制器实现 Subscriber 接口，在所属模块内完成注册。
//
//	type OrderEvents struct{}
//
//	func (e *OrderEvents) Subscribe(r *events.Routes) {
//		r.On("order.created", e.OnOrderCreated)
//	}
package events

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// Subscriber 由需要订阅事件的控制器实现。
type Subscriber interface {
	Subscribe(*Routes)
}

// Routes 收集一个控制器的全部事件订阅。
//
// 链式方法全部返回同一个类型，无需 Build。
type Routes struct {
	receiver any
	items    []*routeItem
}

type routeItem struct {
	event   string
	once    bool
	handler any
}

// On 订阅一个常驻事件处理器。
func (r *Routes) On(event string, handler any) *Routes {
	r.items = append(r.items, &routeItem{event: event, handler: handler})
	return r
}

// Once 订阅一个只执行一次的事件处理器。
func (r *Routes) Once(event string, handler any) *Routes {
	r.items = append(r.items, &routeItem{event: event, once: true, handler: handler})
	return r
}

// bind 把声明绑定到事件总线。
func (r *Routes) bind(bus *Bus) error {
	if r == nil || bus == nil {
		return nil
	}
	for _, item := range r.items {
		if item == nil || strings.TrimSpace(item.event) == "" {
			continue
		}
		handler, err := buildHandler(r.receiver, methodNameOf(r.receiver, item.handler))
		if err != nil {
			return err
		}
		if item.once {
			bus.Once(item.event, handler)
			continue
		}
		bus.On(item.event, handler)
	}
	return nil
}

// methodNameOf 反推方法名。
func methodNameOf(receiver any, handler any) string {
	rv := reflect.ValueOf(receiver)
	hv := reflect.ValueOf(handler)
	if !rv.IsValid() || !hv.IsValid() || hv.Kind() != reflect.Func {
		return ""
	}
	rt := rv.Type()
	for i := 0; i < rt.NumMethod(); i++ {
		if rv.Method(i).Pointer() == hv.Pointer() {
			return rt.Method(i).Name
		}
	}
	return ""
}

// buildHandler 把 receiver 方法包装成 Handler。
func buildHandler(receiver any, methodName string) (Handler, error) {
	if receiver == nil || methodName == "" {
		return nil, fmt.Errorf("invalid events handler: receiver=%T method=%q", receiver, methodName)
	}
	method := reflect.ValueOf(receiver).MethodByName(methodName)
	if !method.IsValid() {
		return nil, fmt.Errorf("events method not found: %s", methodName)
	}
	return func(ctx context.Context, args ...any) error {
		in := make([]reflect.Value, 0, method.Type().NumIn())
		argIndex := 0
		for i := 0; i < method.Type().NumIn(); i++ {
			paramType := method.Type().In(i)
			if paramType == reflect.TypeFor[context.Context]() {
				if ctx == nil {
					ctx = context.Background()
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
