// bind.go 提供 HTTP 上下文到结构体的绑定辅助。
package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// 这组 tag 定义了结构体字段从哪个 HTTP 输入源取值。
// `BindInto` / `BindStrictInto` 会按这些 tag 把请求数据写回 DTO 字段。
const (
	TagParam    = "param"
	TagQuery    = "query"
	TagHeader   = "header"
	TagForm     = "form"
	TagCookie   = "cookie"
	TagBody     = "body"
	TagJSON     = "json"
	TagFile     = "file"
	TagFiles    = "files"
	TagRequired = "required"
)

// Validator 定义对象校验接口（常用于 Bind 完成后对 DTO 做校验）。
type Validator interface {
	Validate(obj any) error
}

// BindInto 会把 HTTP 请求中的数据填充进一个 struct 指针。
//
// 数据来源包括：
// - JSON body（当字段没有 file/files 标签时）
// - 按字段标签读取的 param/query/header/form/cookie/body/file/files
//
// 这是对 v2 行为的一次延续，但读取入口改成了 `runtime.HandlerContext`。
func BindInto(ctx *runtime.HandlerContext, obj any) error {
	return bindInto(ctx, obj, false)
}

// BindStrictInto 是 `Bind[T]` 聚合绑定使用的严格模式版本。
//
// 它只允许这些数据来源：
// - body
// - param
// - query
// - header
//
// 它会拒绝 file/files 标签，因为这类字段通常不应直接放进 DTO 聚合参数中。
func BindStrictInto(ctx *runtime.HandlerContext, obj any) error {
	return bindInto(ctx, obj, true)
}

func bindInto(ctx *runtime.HandlerContext, obj any, strict bool) error {
	if ctx == nil || obj == nil {
		return nil
	}

	val := reflect.ValueOf(obj)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return nil
	}

	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return nil
	}

	typ := elem.Type()

	hasFileTag := false
	for i := 0; i < elem.NumField(); i++ {
		field := typ.Field(i)
		tag := field.Tag
		if tag.Get(TagFile) != "" || tag.Get(TagFiles) != "" {
			if strict {
				return fmt.Errorf("Bind[T] does not allow %q/%q tags", TagFile, TagFiles)
			}
			// file/files 一旦出现，就不再先整包 JSON unmarshal，
			// 避免文件字段与 body JSON 混合时产生“先整体覆盖，再局部覆盖”的歧义。
			hasFileTag = true
			break
		}
	}

	// 当不存在 file/files 标签时，优先尝试用 JSON body 填充整个 struct，
	// 这与 v2 的行为保持一致。
	if !hasFileTag && len(ctx.Request.BodyBytes) > 0 && isJSONRequest(ctx) {
		if err := json.Unmarshal(ctx.Request.BodyBytes, obj); err != nil {
			return fmt.Errorf("invalid json body: %w", err)
		}
	}

	var bodyMap map[string]any
	form := formValues(ctx)
	cookies := cookieValues(ctx)

	for i := 0; i < elem.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := elem.Field(i)
		if !fieldVal.CanSet() {
			continue
		}

		tag := field.Tag
		required := fieldRequired(tag)

		if paramKey := tag.Get(TagParam); paramKey != "" {
			raw := ctx.Request.Params[paramKey]
			if required && raw == "" {
				return fmt.Errorf("missing required param %q for field %s", paramKey, field.Name)
			}
			if err := setFieldValue(fieldVal, raw); err != nil {
				return fmt.Errorf("invalid param %q for field %s: %w", paramKey, field.Name, err)
			}
			continue
		}

		if queryKey := tag.Get(TagQuery); queryKey != "" {
			raw := ctx.Request.Query[queryKey]
			if required && raw == "" {
				return fmt.Errorf("missing required query %q for field %s", queryKey, field.Name)
			}
			if err := setFieldValue(fieldVal, raw); err != nil {
				return fmt.Errorf("invalid query %q for field %s: %w", queryKey, field.Name, err)
			}
			continue
		}

		if formKey := tag.Get(TagForm); formKey != "" {
			raw := ""
			if list := form[formKey]; len(list) > 0 {
				raw = list[0]
			}
			if required && raw == "" {
				return fmt.Errorf("missing required form %q for field %s", formKey, field.Name)
			}
			if err := setFieldValue(fieldVal, raw); err != nil {
				return fmt.Errorf("invalid form %q for field %s: %w", formKey, field.Name, err)
			}
			continue
		}

		if headerKey := tag.Get(TagHeader); headerKey != "" {
			raw := ctx.Request.Headers[headerKey]
			if required && raw == "" {
				return fmt.Errorf("missing required header %q for field %s", headerKey, field.Name)
			}
			if err := setFieldValue(fieldVal, raw); err != nil {
				return fmt.Errorf("invalid header %q for field %s: %w", headerKey, field.Name, err)
			}
			continue
		}

		if cookieKey := tag.Get(TagCookie); cookieKey != "" {
			raw := cookies[cookieKey]
			if required && raw == "" {
				return fmt.Errorf("missing required cookie %q for field %s", cookieKey, field.Name)
			}
			if err := setFieldValue(fieldVal, raw); err != nil {
				return fmt.Errorf("invalid cookie %q for field %s: %w", cookieKey, field.Name, err)
			}
			continue
		}

		if bodyKey := tag.Get(TagBody); bodyKey != "" {
			if bodyMap == nil && len(ctx.Request.BodyBytes) > 0 {
				// 只在第一次遇到 body tag 时解析一次 JSON object，后续字段复用缓存。
				if err := json.Unmarshal(ctx.Request.BodyBytes, &bodyMap); err != nil {
					return fmt.Errorf("invalid json body: %w", err)
				}
			}
			if bodyMap == nil {
				if required {
					return fmt.Errorf("missing required body (no json object) for field %s", field.Name)
				}
				continue
			}
			val, ok := bodyMap[bodyKey]
			if required && (!ok || val == nil) {
				return fmt.Errorf("missing required body key %q for field %s", bodyKey, field.Name)
			}
			if ok {
				if err := setFieldValueFromInterface(fieldVal, val); err != nil {
					return fmt.Errorf("invalid body key %q for field %s: %w", bodyKey, field.Name, err)
				}
			}
			continue
		}

		if fileKey := tag.Get(TagFile); fileKey != "" {
			file := firstUploadedFile(ctx, fileKey)
			if required && file == nil {
				return fmt.Errorf("missing required file %q for field %s", fileKey, field.Name)
			}
			if fieldVal.Type() == reflect.TypeFor[*UploadedFile]() {
				fieldVal.Set(reflect.ValueOf(file))
			} else if fieldVal.Type() == reflect.TypeOf(UploadedFile{}) && file != nil {
				fieldVal.Set(reflect.ValueOf(*file))
			}
			continue
		}

		if filesKey := tag.Get(TagFiles); filesKey != "" {
			files := uploadedFiles(ctx, filesKey)
			if required && len(files) == 0 {
				return fmt.Errorf("missing required files %q for field %s", filesKey, field.Name)
			}
			if fieldVal.Kind() == reflect.Slice && fieldVal.Type().Elem() == reflect.TypeFor[*UploadedFile]() {
				fieldVal.Set(reflect.ValueOf(files))
			}
			continue
		}
	}

	return nil
}

// MustBindInto 是 BindInto 的 panic 版本。
// 它主要给极少数明确接受 panic 风格的兼容场景保留，新代码更推荐返回 error。
func MustBindInto(ctx *runtime.HandlerContext, obj any) {
	if err := BindInto(ctx, obj); err != nil {
		panic(err)
	}
}

func firstUploadedFile(ctx *runtime.HandlerContext, key string) *UploadedFile {
	filesAny, _ := ctx.Get(MetadataKeyFiles)
	files, _ := filesAny.(map[string][]*UploadedFile)
	if files == nil {
		return nil
	}
	list := files[key]
	if len(list) == 0 {
		return nil
	}
	return list[0]
}

func uploadedFiles(ctx *runtime.HandlerContext, key string) []*UploadedFile {
	filesAny, _ := ctx.Get(MetadataKeyFiles)
	files, _ := filesAny.(map[string][]*UploadedFile)
	if files == nil {
		return nil
	}
	return files[key]
}

func setFieldValue(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value == "" {
			field.SetInt(0)
			return nil
		}
		i, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if value == "" {
			field.SetUint(0)
			return nil
		}
		i, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(i)
	case reflect.Float32, reflect.Float64:
		if value == "" {
			field.SetFloat(0)
			return nil
		}
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(f)
	case reflect.Bool:
		if value == "" {
			field.SetBool(false)
			return nil
		}
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)
	}
	return nil
}

// setFieldValueFromInterface 主要服务 `body:"field"` 这种“先解成 map，再按字段取值”的路径。
// 这里故意只支持少量基础类型和 map，复杂对象统一交给整包 JSON 解码。
func setFieldValueFromInterface(field reflect.Value, value any) error {
	if value == nil {
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		if s, ok := value.(string); ok {
			field.SetString(s)
		} else {
			field.SetString(fmt.Sprintf("%v", value))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch v := value.(type) {
		case float64:
			field.SetInt(int64(v))
		case int:
			field.SetInt(int64(v))
		case int64:
			field.SetInt(v)
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return err
			}
			field.SetInt(i)
		default:
			return fmt.Errorf("cannot convert %T to int", value)
		}
	case reflect.Float32, reflect.Float64:
		switch v := value.(type) {
		case float64:
			field.SetFloat(v)
		case int:
			field.SetFloat(float64(v))
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}
			field.SetFloat(f)
		default:
			return fmt.Errorf("cannot convert %T to float", value)
		}
	case reflect.Bool:
		switch v := value.(type) {
		case bool:
			field.SetBool(v)
		case string:
			b, err := strconv.ParseBool(v)
			if err != nil {
				return err
			}
			field.SetBool(b)
		default:
			return fmt.Errorf("cannot convert %T to bool", value)
		}
	case reflect.Map:
		if m, ok := value.(map[string]any); ok {
			mapVal := reflect.MakeMap(field.Type())
			for k, v := range m {
				keyVal := reflect.ValueOf(k)
				valVal := reflect.ValueOf(v)
				if valVal.IsValid() && valVal.Type().AssignableTo(field.Type().Elem()) {
					mapVal.SetMapIndex(keyVal, valVal)
				}
			}
			field.Set(mapVal)
			return nil
		}
		return fmt.Errorf("cannot convert %T to map", value)
	}

	return nil
}

func isRequiredTag(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "required":
		return true
	default:
		// 这里沿用历史上的“只要写了 required 标签就视为 required”的宽松兼容行为。
		return true
	}
}

func isJSONRequest(ctx *runtime.HandlerContext) bool {
	if ctx == nil {
		return false
	}
	if req, ok := ctx.Request.Raw.(*http.Request); ok && req != nil {
		ct := strings.ToLower(req.Header.Get("Content-Type"))
		return strings.HasPrefix(ct, "application/json") || strings.Contains(ct, "+json")
	}
	return false
}

func formValues(ctx *runtime.HandlerContext) map[string][]string {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Get(MetadataKeyForm); ok {
		if m, ok := v.(map[string][]string); ok {
			return m
		}
	}
	return nil
}

func cookieValues(ctx *runtime.HandlerContext) map[string]string {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Get(MetadataKeyCookies); ok {
		if m, ok := v.(map[string]string); ok {
			return m
		}
	}
	out := make(map[string]string)
	if req, ok := ctx.Request.Raw.(*http.Request); ok && req != nil {
		for _, c := range req.Cookies() {
			if c != nil {
				out[c.Name] = c.Value
			}
		}
	}
	// 这里不主动回写 metadata，避免和请求入口是否已经缓存 cookie 的策略耦合。
	return out
}
