// execution.go 实现 GraphQL adapter 的字段执行与结果拼装流程。
package graphql

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	gqlbinding "github.com/sao-lang/lania-g/protocol/graphql/v3/binding"
	gqlprotocol "github.com/sao-lang/lania-g/protocol/graphql/v3/protocol"

	gqlast "github.com/graphql-go/graphql/language/ast"
	gqlparser "github.com/graphql-go/graphql/language/parser"
	gqlsource "github.com/graphql-go/graphql/language/source"
)

func (a *Adapter) executeRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, req *GraphQLRequest) GraphQLResponse {
	doc, err := gqlparser.Parse(gqlparser.ParseParams{
		Source: &gqlsource.Source{Body: []byte(req.Query), Name: "GraphQL request"},
	})
	if err != nil {
		return GraphQLResponse{Errors: []any{a.formatError(err)}}
	}

	op, fragments, err := findOperationAndFragments(doc, req.OperationName)
	if err != nil {
		return GraphQLResponse{Errors: []any{a.formatError(err)}}
	}

	vars, err := coerceVariables(op.VariableDefinitions, req.Variables)
	if err != nil {
		return GraphQLResponse{Errors: []any{a.formatError(err)}}
	}

	opCtx := &OperationContext{
		Context:      ctx,
		HTTPRequest:  r,
		HTTPResponse: w,
		Request:      req,
		Operation:    op,
		Schema:       a.schemaSnapshot(),
		Fragments:    fragments,
		Variables:    vars,
		Response:     &GraphQLResponse{},
	}
	if opCtx.Schema == nil {
		return GraphQLResponse{Errors: []any{a.formatError(fmt.Errorf("graphql schema is not available"))}}
	}

	for _, ext := range opCtx.Schema.Extensions {
		if ext != nil {
			if err := ext.BeforeOperation(opCtx); err != nil {
				return GraphQLResponse{Errors: []any{a.formatError(err)}}
			}
		}
	}

	rootType := rootTypeNameForOperation(op.Operation)
	if _, err := validateSelectionSet(opCtx, rootType, op.SelectionSet); err != nil {
		return GraphQLResponse{Errors: []any{a.formatError(err)}}
	}

	data := a.executeSelectionSet(opCtx, rootType, op.SelectionSet, nil, nil)
	resp := GraphQLResponse{Data: data}
	if len(opCtx.Errors) > 0 {
		resp.Errors = make([]any, 0, len(opCtx.Errors))
		for _, item := range opCtx.Errors {
			resp.Errors = append(resp.Errors, a.formatError(item))
		}
	}
	if req.Extensions != nil {
		resp.Extensions = req.Extensions
	}
	for _, ext := range opCtx.Schema.Extensions {
		if ext != nil {
			ext.AfterOperation(opCtx, &resp)
		}
	}
	return resp
}

func (a *Adapter) executeSelectionSet(opCtx *OperationContext, parentType string, set *gqlast.SelectionSet, root any, path []string) map[string]any {
	fields, err := collectFields(opCtx, parentType, set)
	if err != nil {
		a.addError(opCtx, err, path)
		return nil
	}
	out := make(map[string]any, len(fields))
	for _, field := range fields {
		key := fieldKey(field)
		nextPath := appendPath(path, key)
		value, err := a.executeField(opCtx, parentType, field, root, nextPath)
		if err != nil {
			a.addError(opCtx, err, nextPath)
			out[key] = nil
			continue
		}
		out[key] = value
	}
	return out
}

func (a *Adapter) executeField(opCtx *OperationContext, parentType string, field *gqlast.Field, root any, path []string) (any, error) {
	if field == nil || field.Name == nil {
		return nil, nil
	}
	switch field.Name.Value {
	case "__typename":
		return parentType, nil
	case "__schema":
		if parentType != string(FieldTypeQuery) || opCtx.Schema.DisableIntrospection {
			return nil, fmt.Errorf("introspection is disabled")
		}
		return projectSelection(opCtx.Schema.describeSchema(), field.SelectionSet), nil
	case "__type":
		if parentType != string(FieldTypeQuery) || opCtx.Schema.DisableIntrospection {
			return nil, fmt.Errorf("introspection is disabled")
		}
		args, err := argumentsToMap(field.Arguments, opCtx.Variables)
		if err != nil {
			return nil, err
		}
		typeName, _ := args["name"].(string)
		return projectSelection(opCtx.Schema.describeTypeByName(typeName), field.SelectionSet), nil
	}

	if opCtx.Schema.object(parentType) == nil {
		return nil, fmt.Errorf("graphql type %s not found", parentType)
	}
	schemaField := opCtx.Schema.field(parentType, field.Name.Value)
	if schemaField == nil {
		return nil, fmt.Errorf("graphql field %s.%s not found", parentType, field.Name.Value)
	}
	if schemaField.Definition == nil || schemaField.Definition.HandlerName == "" {
		return a.completeFieldValue(opCtx, parentType, schemaField, field, extractFieldValue(reflect.ValueOf(root), field.Name.Value), path)
	}

	args, err := coerceFieldArguments(schemaField, field.Arguments, opCtx.Variables)
	if err != nil {
		return nil, err
	}

	info := &ExecutionInfo{
		Field:         field,
		ParentType:    parentType,
		ReturnType:    schemaField.ReturnType.Name,
		Path:          append([]string{}, path...),
		OperationName: opCtx.Request.OperationName,
	}
	value, err := a.executeRuntimeField(opCtx, parentType, field, root, args, info)
	if err != nil {
		return nil, err
	}
	return a.completeFieldValue(opCtx, parentType, schemaField, field, value, path)
}

func (a *Adapter) completeFieldValue(opCtx *OperationContext, parentType string, schemaField *compiledField, field *gqlast.Field, value any, path []string) (any, error) {
	if field.SelectionSet == nil || len(field.SelectionSet.Selections) == 0 {
		return value, nil
	}
	if schemaField.ReturnType == nil || schemaField.ReturnType.Scalar {
		return projectSelection(value, field.SelectionSet), nil
	}
	if schemaField.ReturnType.List {
		rv := reflect.ValueOf(value)
		for rv.IsValid() && rv.Kind() == reflect.Ptr {
			if rv.IsNil() {
				return nil, nil
			}
			rv = rv.Elem()
		}
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return nil, fmt.Errorf("graphql field %s.%s expected list result", parentType, field.Name.Value)
		}
		out := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			item := rv.Index(i).Interface()
			if opCtx.Schema.object(schemaField.ReturnType.Name) != nil {
				out = append(out, a.executeSelectionSet(opCtx, schemaField.ReturnType.Name, field.SelectionSet, item, path))
			} else {
				out = append(out, projectSelection(item, field.SelectionSet))
			}
		}
		return out, nil
	}
	if opCtx.Schema.object(schemaField.ReturnType.Name) != nil {
		return a.executeSelectionSet(opCtx, schemaField.ReturnType.Name, field.SelectionSet, value, path), nil
	}
	return projectSelection(value, field.SelectionSet), nil
}

func (a *Adapter) executeRuntimeField(opCtx *OperationContext, parentType string, field *gqlast.Field, root any, args map[string]any, info *ExecutionInfo) (any, error) {
	hctx := runtime.AcquireHandlerContext(gqlprotocol.Protocol)
	defer runtime.ReleaseHandlerContext(hctx)
	hctx.WithContext(opCtx.Context)
	hctx.Request.Method = parentType
	hctx.Request.Path = field.Name.Value
	hctx.Request.Raw = opCtx.HTTPRequest
	hctx.Response.Raw = opCtx.HTTPResponse
	copyHeaderValues(hctx.Request.Headers, hctx.Request.HeadersMulti, opCtx.HTTPRequest.Header)
	hctx.Request.Headers["Host"] = opCtx.HTTPRequest.Host

	gctx := a.newGraphQLContext(opCtx, field, parentType, root, args, info)
	gctx.AttachHandlerContext(hctx)
	hctx.Set(gqlbinding.MetadataKeyContext, gctx)
	hctx.Set(gqlbinding.MetadataKeyHeaders, opCtx.HTTPRequest.Header.Clone())
	hctx.Set(gqlbinding.MetadataKeyVars, copyStringAnyMap(opCtx.Variables))
	hctx.Set(gqlbinding.MetadataKeyRoot, root)
	hctx.Set(gqlbinding.MetadataKeyInfo, info)
	hctx.Set(gqlbinding.MetadataKeyField, args)
	hctx.Set(gqlbinding.MetadataKeyFieldTyp, parentType)
	hctx.Set(gqlbinding.MetadataKeyFieldName, field.Name.Value)
	hctx.Set(gqlbinding.MetadataKeySelectionSet, gqlbinding.SelectionSet{
		Raw:    field.SelectionSet,
		Fields: selectionFieldNames(field.SelectionSet),
	})
	hctx.Set(gqlbinding.MetadataKeyOperationName, opCtx.Request.OperationName)
	hctx.Set(gqlbinding.MetadataKeyRawQuery, opCtx.Request.Query)
	hctx.Set(gqlbinding.MetadataKeyExtensions, copyStringAnyMap(opCtx.Request.Extensions))
	hctx.Set(gqlbinding.MetadataKeySession, map[string]any{})
	if a.validator != nil {
		hctx.Set(gqlbinding.MetadataKeyValidator, a.validator)
	}

	return a.host.Runtime().Execute(hctx)
}

func (a *Adapter) newGraphQLContext(opCtx *OperationContext, field *gqlast.Field, parentType string, root any, args map[string]any, info *ExecutionInfo) *gqlbinding.GraphQLContext {
	var gctx *gqlbinding.GraphQLContext
	if a.contextFactory != nil {
		gctx = a.contextFactory(opCtx.HTTPRequest)
	}
	if gctx == nil {
		gctx = &gqlbinding.GraphQLContext{}
	}
	gqlbinding.InitContext(
		gctx,
		opCtx.Context,
		opCtx.HTTPRequest,
		opCtx.HTTPResponse,
		opCtx.Request.OperationName,
		opCtx.Request.Query,
		parentType,
		field.Name.Value,
		append([]string{}, info.Path...),
		field.SelectionSet,
		root,
		info,
		copyStringAnyMap(opCtx.Variables),
		headerSnapshot(opCtx.HTTPRequest.Header),
		copyStringAnyMap(opCtx.Request.Extensions),
		map[string]any{},
		copyStringAnyMap(args),
	)
	return gctx
}
