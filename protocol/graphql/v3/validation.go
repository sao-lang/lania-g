// validation.go 实现 GraphQL 请求与 schema 的校验逻辑。
package graphql

import (
	"fmt"

	gqlast "github.com/graphql-go/graphql/language/ast"
)

func findOperationAndFragments(doc *gqlast.Document, operationName string) (*gqlast.OperationDefinition, map[string]*gqlast.FragmentDefinition, error) {
	var first *gqlast.OperationDefinition
	fragments := make(map[string]*gqlast.FragmentDefinition)
	for _, def := range doc.Definitions {
		switch typed := def.(type) {
		case *gqlast.OperationDefinition:
			if first == nil {
				first = typed
			}
			if operationName != "" && typed.Name != nil && typed.Name.Value == operationName {
				first = typed
			}
		case *gqlast.FragmentDefinition:
			if typed.Name != nil {
				fragments[typed.Name.Value] = typed
			}
		}
	}
	if operationName != "" {
		if first == nil || first.Name == nil || first.Name.Value != operationName {
			return nil, nil, fmt.Errorf("operation %q not found", operationName)
		}
	}
	if first == nil {
		return nil, nil, fmt.Errorf("operation not found")
	}
	return first, fragments, nil
}

func validateSelectionSet(opCtx *OperationContext, parentType string, set *gqlast.SelectionSet) (int, error) {
	fields, err := collectFields(opCtx, parentType, set)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, field := range fields {
		if field == nil || field.Name == nil {
			continue
		}
		switch field.Name.Value {
		case "__typename":
			total++
			continue
		case "__schema":
			if parentType != string(FieldTypeQuery) || opCtx.Schema.DisableIntrospection {
				return 0, fmt.Errorf("introspection is disabled")
			}
			total++
			continue
		case "__type":
			if parentType != string(FieldTypeQuery) || opCtx.Schema.DisableIntrospection {
				return 0, fmt.Errorf("introspection is disabled")
			}
			total++
			continue
		}
		if opCtx.Schema.object(parentType) == nil {
			return 0, fmt.Errorf("graphql type %s not found", parentType)
		}
		schemaField := opCtx.Schema.field(parentType, field.Name.Value)
		if schemaField == nil {
			return 0, fmt.Errorf("graphql field %s.%s not found", parentType, field.Name.Value)
		}
		if _, err := coerceFieldArguments(schemaField, field.Arguments, opCtx.Variables); err != nil {
			return 0, err
		}
		fieldComplexity := maxInt(1, schemaField.Complexity)
		if schemaField.ReturnType != nil && !schemaField.ReturnType.Scalar {
			if field.SelectionSet == nil || len(field.SelectionSet.Selections) == 0 {
				return 0, fmt.Errorf("graphql field %s.%s requires selection set", parentType, field.Name.Value)
			}
			child, err := validateSelectionSet(opCtx, schemaField.ReturnType.Name, field.SelectionSet)
			if err != nil {
				return 0, err
			}
			fieldComplexity += child
		} else if field.SelectionSet != nil && len(field.SelectionSet.Selections) > 0 {
			return 0, fmt.Errorf("graphql field %s.%s does not support selection set", parentType, field.Name.Value)
		}
		total += fieldComplexity
	}
	if opCtx.Schema.ComplexityLimit > 0 && total > opCtx.Schema.ComplexityLimit {
		return 0, fmt.Errorf("graphql complexity %d exceeds limit %d", total, opCtx.Schema.ComplexityLimit)
	}
	return total, nil
}

func collectFields(opCtx *OperationContext, parentType string, set *gqlast.SelectionSet) ([]*gqlast.Field, error) {
	if set == nil {
		return nil, nil
	}
	out := make([]*gqlast.Field, 0, len(set.Selections))
	for _, selection := range set.Selections {
		switch item := selection.(type) {
		case *gqlast.Field:
			include, err := shouldIncludeDirectives(item.Directives, opCtx.Variables)
			if err != nil {
				return nil, err
			}
			if include {
				out = append(out, item)
			}
		case *gqlast.FragmentSpread:
			include, err := shouldIncludeDirectives(item.Directives, opCtx.Variables)
			if err != nil {
				return nil, err
			}
			if !include || item.Name == nil {
				continue
			}
			fragment := opCtx.Fragments[item.Name.Value]
			if fragment == nil || !fragmentMatchesType(parentType, fragment.TypeCondition) {
				continue
			}
			child, err := collectFields(opCtx, parentType, fragment.SelectionSet)
			if err != nil {
				return nil, err
			}
			out = append(out, child...)
		case *gqlast.InlineFragment:
			include, err := shouldIncludeDirectives(item.Directives, opCtx.Variables)
			if err != nil {
				return nil, err
			}
			if !include || !fragmentMatchesType(parentType, item.TypeCondition) {
				continue
			}
			child, err := collectFields(opCtx, parentType, item.SelectionSet)
			if err != nil {
				return nil, err
			}
			out = append(out, child...)
		}
	}
	return out, nil
}

func fragmentMatchesType(parentType string, cond *gqlast.Named) bool {
	if cond == nil || cond.Name == nil {
		return true
	}
	return cond.Name.Value == parentType
}

func shouldIncludeDirectives(directives []*gqlast.Directive, vars map[string]any) (bool, error) {
	include := true
	for _, directive := range directives {
		if directive == nil || directive.Name == nil {
			continue
		}
		args, err := argumentsToMap(directive.Arguments, vars)
		if err != nil {
			return false, err
		}
		switch directive.Name.Value {
		case "skip":
			if v, ok := args["if"].(bool); ok && v {
				include = false
			}
		case "include":
			if v, ok := args["if"].(bool); ok && !v {
				include = false
			}
		}
	}
	return include, nil
}
