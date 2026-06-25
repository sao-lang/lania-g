// composite.go 实现 HTTP composite struct 的组合绑定逻辑。
package http

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// CompositeStruct 允许 handler 把多个 wrapper 聚合进一个 struct 参数。
// 例如：
//
//	func (c *C) H(args struct {
//		Body Body[*Req]
//		ID   Param[string] `param:"id"`
//	}) ...
//
// 这个模式和 AutoStruct 的区别是：
// - AutoStruct 把整个 struct 当成一个普通 DTO 统一绑定
// - CompositeStruct 则逐字段识别 wrapper，并分别调用对应 resolver
func matchCompositeStruct(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	original := t
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return runtime.WrapperDescriptor{}, false
	}

	// 本包自己的 wrapper 不能再被当作 composite 容器，否则会出现递归/误匹配。
	if t.PkgPath() == PackagePath() {
		return runtime.WrapperDescriptor{}, false
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if isSupportedCompositeFieldType(f.Type) {
			return runtime.WrapperDescriptor{
				Kind:        "CompositeStruct",
				WrapperType: original,
				InnerType:   t,
			}, true
		}
	}

	return runtime.WrapperDescriptor{}, false
}

func resolveCompositeStruct(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	t := desc.WrapperType
	ptr := false
	if t.Kind() == reflect.Ptr {
		ptr = true
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("composite args must be struct or *struct, got %s", desc.WrapperType.String())
	}

	out := reflect.New(t).Elem()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		fv := out.Field(i)
		if !fv.CanSet() {
			continue
		}

		value, ok, err := resolveCompositeField(ctx, f)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		vv := reflect.ValueOf(value)
		if vv.IsValid() && vv.Type().AssignableTo(fv.Type()) {
			fv.Set(vv)
		} else if vv.IsValid() && vv.Type().ConvertibleTo(fv.Type()) {
			fv.Set(vv.Convert(fv.Type()))
		}
	}

	if ptr {
		ptrVal := reflect.New(t)
		ptrVal.Elem().Set(out)
		return ptrVal.Interface(), nil
	}
	return out.Interface(), nil
}

// resolveCompositeField 按字段类型分发到具体 resolver。
// 它是 CompositeStruct 的核心：用字段级 wrapper 语义替代整对象绑定。
func resolveCompositeField(ctx *runtime.HandlerContext, field reflect.StructField) (any, bool, error) {
	ft := field.Type
	if ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}

	if field.Type == reflect.TypeFor[*HttpContext]() || field.Type == reflect.TypeFor[Context]() {
		val, err := resolveHTTPContext(ctx, runtime.WrapperDescriptor{WrapperType: field.Type})
		return val, true, err
	}

	// `Cookies` 是一个命名 map 类型，不走通用 wrapper struct 识别。
	if field.Type == reflect.TypeOf(Cookies(nil)) {
		val, err := resolveCookies(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return val, true, err
	}

	if ft.Kind() != reflect.Struct || ft.PkgPath() != PackagePath() {
		return nil, false, nil
	}

	name := trimGenericName(ft.Name())
	desc := runtime.WrapperDescriptor{WrapperType: ft}
	if vField, ok := ft.FieldByName("Value"); ok {
		desc.InnerType = vField.Type
	}

	required := fieldRequired(field.Tag)

	switch name {
	case "Body", "BodyAs", "MustBodyAs":
		if required && len(ctx.Request.BodyBytes) == 0 && ctx.Request.Body == nil {
			return nil, true, fmt.Errorf("missing required body for field %s", field.Name)
		}
		v, err := resolveBody(ctx, desc)
		return v, true, err
	case "Query":
		if key := field.Tag.Get(TagQuery); key != "" {
			desc.BindingName = key
		}
		// CompositeStruct 没有路由模板可以自动推断 query/header/cookie 名称，
		// required 场景下必须通过 tag 显式给出 bindingName。
		if required && desc.BindingName == "" {
			return nil, true, fmt.Errorf("required query bindingName missing: add %q tag for field %s", TagQuery, field.Name)
		}
		if required && desc.BindingName != "" && ctx.Request.Query[desc.BindingName] == "" {
			return nil, true, fmt.Errorf("missing required query %q for field %s", desc.BindingName, field.Name)
		}
		v, err := resolveQuery(ctx, desc)
		return v, true, err
	case "Param":
		if key := field.Tag.Get(TagParam); key != "" {
			desc.BindingName = key
		}
		if required && desc.BindingName == "" {
			return nil, true, fmt.Errorf("required param bindingName missing: add %q tag for field %s", TagParam, field.Name)
		}
		if required && desc.BindingName != "" && ctx.Request.Params[desc.BindingName] == "" {
			return nil, true, fmt.Errorf("missing required param %q for field %s", desc.BindingName, field.Name)
		}
		v, err := resolveParam(ctx, desc)
		return v, true, err
	case "Header":
		if key := field.Tag.Get(TagHeader); key != "" {
			desc.BindingName = key
		}
		if required && desc.BindingName == "" {
			return nil, true, fmt.Errorf("required header bindingName missing: add %q tag for field %s", TagHeader, field.Name)
		}
		if required && desc.BindingName != "" && ctx.Request.Headers[desc.BindingName] == "" {
			return nil, true, fmt.Errorf("missing required header %q for field %s", desc.BindingName, field.Name)
		}
		v, err := resolveHeader(ctx, desc)
		return v, true, err
	case "Form":
		if key := field.Tag.Get(TagForm); key != "" {
			desc.BindingName = key
		}
		if required && desc.BindingName == "" {
			return nil, true, fmt.Errorf("required form bindingName missing: add %q tag for field %s", TagForm, field.Name)
		}
		v, err := resolveForm(ctx, desc)
		return v, true, err
	case "Cookie":
		if key := field.Tag.Get(TagCookie); key != "" {
			desc.BindingName = key
		}
		if required && desc.BindingName == "" {
			return nil, true, fmt.Errorf("required cookie bindingName missing: add %q tag for field %s", TagCookie, field.Name)
		}
		v, err := resolveCookie(ctx, desc)
		// resolveCookie 缺失时只会给零值 wrapper，因此 required 约束要在这里补一层检查。
		if required {
			// Cookie[T] 是 wrapper struct，这里通过反射看 `Value` 字段是否仍是零值。
			rv := reflect.ValueOf(v)
			if rv.IsValid() && rv.Kind() == reflect.Struct {
				f := rv.FieldByName("Value")
				if f.IsValid() && f.IsZero() {
					return nil, true, fmt.Errorf("missing required cookie %q for field %s", desc.BindingName, field.Name)
				}
			}
		}
		return v, true, err
	case "Bind":
		v, err := resolveBind(ctx, desc)
		return v, true, err
	case "MustBind":
		v, err := resolveMustBind(ctx, desc)
		return v, true, err
	case "File":
		if key := field.Tag.Get(TagFile); key != "" {
			desc.BindingName = key
		}
		if required && desc.BindingName == "" {
			return nil, true, fmt.Errorf("required file bindingName missing: add %q tag for field %s", TagFile, field.Name)
		}
		v, err := resolveFile(ctx, desc)
		// resolveFile 缺失时返回 `File{Value:nil}`，required 校验同样放在 composite 层兜底。
		if required {
			if fv, ok := v.(File); ok && fv.Value == nil {
				return nil, true, fmt.Errorf("missing required file %q for field %s", desc.BindingName, field.Name)
			}
		}
		return v, true, err
	case "Files":
		if key := field.Tag.Get(TagFiles); key != "" {
			desc.BindingName = key
		}
		if required && desc.BindingName == "" {
			return nil, true, fmt.Errorf("required files bindingName missing: add %q tag for field %s", TagFiles, field.Name)
		}
		v, err := resolveFiles(ctx, desc)
		if required {
			if fv, ok := v.(Files); ok && len(fv.Value) == 0 {
				return nil, true, fmt.Errorf("missing required files %q for field %s", desc.BindingName, field.Name)
			}
		}
		return v, true, err
	default:
		return nil, false, nil
	}
}

func isSupportedCompositeFieldType(t reflect.Type) bool {
	if t == reflect.TypeFor[*HttpContext]() || t == reflect.TypeFor[Context]() {
		return true
	}
	if t == reflect.TypeOf(Cookies(nil)) {
		return true
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	if t.PkgPath() != PackagePath() {
		return false
	}
	switch trimGenericName(t.Name()) {
	case "Body", "Query", "Param", "Header", "Form", "Cookie", "Bind", "MustBind", "BodyAs", "MustBodyAs", "File", "Files":
		return true
	default:
		return false
	}
}
