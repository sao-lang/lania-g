// coercion.go 实现 GraphQL 标量与输入输出值的转换辅助。
package graphql

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	gqlast "github.com/graphql-go/graphql/language/ast"
)

func coerceVariables(defs []*gqlast.VariableDefinition, provided map[string]any) (map[string]any, error) {
	out := copyStringAnyMap(provided)
	if out == nil {
		out = map[string]any{}
	}
	for _, def := range defs {
		if def == nil || def.Variable == nil || def.Variable.Name == nil {
			continue
		}
		name := def.Variable.Name.Value
		value, exists := out[name]
		if !exists && def.DefaultValue != nil {
			defaultValue, err := valueFromAST(def.DefaultValue, out)
			if err != nil {
				return nil, err
			}
			out[name] = defaultValue
			value = defaultValue
			exists = true
		}
		if isNonNullType(def.Type) && !exists {
			return nil, fmt.Errorf("variable %q is required", name)
		}
		if exists && !matchesGraphQLType(def.Type, value) {
			return nil, fmt.Errorf("variable %q does not match declared GraphQL type", name)
		}
	}
	return out, nil
}

func coerceFieldArguments(field *compiledField, args []*gqlast.Argument, vars map[string]any) (map[string]any, error) {
	if field == nil {
		return nil, fmt.Errorf("graphql field schema is nil")
	}
	out := make(map[string]any, len(args))
	for _, arg := range args {
		if arg == nil || arg.Name == nil {
			continue
		}
		value, err := valueFromAST(arg.Value, vars)
		if err != nil {
			return nil, err
		}
		if len(field.Args) > 0 {
			if _, ok := field.Args[arg.Name.Value]; !ok {
				return nil, fmt.Errorf("graphql field %s.%s does not accept arg %q", field.ParentType, field.Name, arg.Name.Value)
			}
		}
		out[arg.Name.Value] = value
	}
	for name, def := range field.Args {
		if _, ok := out[name]; !ok && def.DefaultValue != nil {
			out[name] = def.DefaultValue
		}
		if def.NonNull {
			if value, ok := out[name]; !ok || value == nil {
				return nil, fmt.Errorf("graphql field %s.%s requires arg %q", field.ParentType, field.Name, name)
			}
		}
	}
	return out, nil
}

func rootTypeNameForOperation(op string) string {
	switch strings.ToLower(op) {
	case "mutation":
		return string(FieldTypeMutation)
	default:
		return string(FieldTypeQuery)
	}
}

func argumentsToMap(args []*gqlast.Argument, vars map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(args))
	for _, arg := range args {
		if arg == nil || arg.Name == nil {
			continue
		}
		value, err := valueFromAST(arg.Value, vars)
		if err != nil {
			return nil, err
		}
		out[arg.Name.Value] = value
	}
	return out, nil
}

func valueFromAST(node gqlast.Value, vars map[string]any) (any, error) {
	switch v := node.(type) {
	case *gqlast.StringValue:
		return v.Value, nil
	case *gqlast.BooleanValue:
		return v.Value, nil
	case *gqlast.IntValue:
		i, err := strconv.ParseInt(v.Value, 10, 64)
		if err != nil {
			return nil, err
		}
		return i, nil
	case *gqlast.FloatValue:
		f, err := strconv.ParseFloat(v.Value, 64)
		if err != nil {
			return nil, err
		}
		return f, nil
	case *gqlast.EnumValue:
		return v.Value, nil
	case *gqlast.Variable:
		if v.Name == nil {
			return nil, nil
		}
		return vars[v.Name.Value], nil
	case *gqlast.ListValue:
		out := make([]any, 0, len(v.Values))
		for _, item := range v.Values {
			value, err := valueFromAST(item, vars)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	case *gqlast.ObjectValue:
		out := make(map[string]any, len(v.Fields))
		for _, field := range v.Fields {
			value, err := valueFromAST(field.Value, vars)
			if err != nil {
				return nil, err
			}
			out[field.Name.Value] = value
		}
		return out, nil
	default:
		return nil, nil
	}
}

func isNonNullType(t gqlast.Type) bool {
	_, ok := t.(*gqlast.NonNull)
	return ok
}

func matchesGraphQLType(t gqlast.Type, value any) bool {
	if value == nil {
		return !isNonNullType(t)
	}
	switch typed := t.(type) {
	case *gqlast.NonNull:
		return matchesGraphQLType(typed.Type, value)
	case *gqlast.List:
		rv := reflect.ValueOf(value)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return false
		}
		for i := 0; i < rv.Len(); i++ {
			if !matchesGraphQLType(typed.Type, rv.Index(i).Interface()) {
				return false
			}
		}
		return true
	default:
		return true
	}
}
