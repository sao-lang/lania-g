// schema_reflect.go 实现 swagger 集成基于反射的 schema 推导逻辑。
package swagger

import (
	"reflect"
	"strconv"
	"strings"
)

func inferSchema(typ reflect.Type) *Schema {
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	schema := &Schema{}
	switch typ.Kind() {
	case reflect.Struct:
		schema.Type = "object"
		schema.Properties = make(map[string]*Schema)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if !field.IsExported() {
				continue
			}
			if ignoredField(field) {
				continue
			}
			name, omitEmpty, skip := jsonFieldName(field)
			if skip {
				continue
			}
			prop := inferSchema(field.Type)
			applySchemaTags(prop, field.Tag)
			if field.Type.Kind() == reflect.Ptr {
				prop.Nullable = true
			}
			schema.Properties[name] = prop
			if isRequiredField(field, omitEmpty) {
				schema.Required = append(schema.Required, name)
			}
		}
	case reflect.Slice, reflect.Array:
		schema.Type = "array"
		schema.Items = inferSchema(typ.Elem())
	case reflect.Map:
		schema.Type = "object"
		schema.AdditionalProperties = inferSchema(typ.Elem())
	case reflect.Bool:
		schema.Type = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		schema.Type = "integer"
		schema.Format = "int32"
	case reflect.Int64:
		schema.Type = "integer"
		schema.Format = "int64"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		schema.Type = "integer"
		schema.Format = "uint32"
	case reflect.Uint64:
		schema.Type = "integer"
		schema.Format = "uint64"
	case reflect.Float32:
		schema.Type = "number"
		schema.Format = "float"
	case reflect.Float64:
		schema.Type = "number"
		schema.Format = "double"
	case reflect.Interface:
		schema.Type = "object"
	default:
		schema.Type = "string"
	}
	return schema
}

func ignoredField(field reflect.StructField) bool {
	for _, tag := range []string{"swagger", "openapi"} {
		if field.Tag.Get(tag) == "-" {
			return true
		}
	}
	return false
}

func applySchemaTags(schema *Schema, tag reflect.StructTag) {
	if schema == nil {
		return
	}
	if desc := tag.Get("description"); desc != "" {
		schema.Description = desc
	}
	if example := tag.Get("example"); example != "" {
		schema.Example = example
	}
	if def := tag.Get("default"); def != "" {
		schema.Default = coerceTagValue(schema.Type, def)
	}
	if enumTag := tag.Get("enum"); enumTag != "" {
		parts := strings.Split(enumTag, "|")
		schema.Enum = make([]interface{}, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			schema.Enum = append(schema.Enum, coerceTagValue(schema.Type, part))
		}
	}
}

func jsonFieldName(field reflect.StructField) (name string, omitEmpty bool, skip bool) {
	name = field.Name
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return name, false, false
	}
	parts := strings.Split(tag, ",")
	if len(parts) > 0 && parts[0] != "" {
		name = parts[0]
	}
	for _, part := range parts[1:] {
		if part == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}

func isRequiredField(field reflect.StructField, omitEmpty bool) bool {
	if requiredTag := field.Tag.Get("required"); requiredTag != "" {
		return requiredTag == "true" || requiredTag == "1"
	}
	return !omitEmpty
}

func coerceTagValue(schemaType, value string) interface{} {
	switch schemaType {
	case "boolean":
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	case "integer":
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	case "number":
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return value
}
