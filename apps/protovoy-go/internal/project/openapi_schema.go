package project

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
)

func (references *openAPIReferences) generateBody(raw json.RawMessage) (json.RawMessage, error) {
	budget := 4096
	value, err := references.schemaValue(raw, 0, &budget)
	if err != nil || value == nil {
		return nil, err
	}
	body, err := json.Marshal(value)
	if err != nil || len(body) > MaxBytes {
		return nil, errors.New("OpenAPI generated JSON body exceeds the import limit")
	}
	return body, nil
}

func (references *openAPIReferences) schemaValue(raw json.RawMessage, depth int, budget *int) (any, error) {
	switch string(raw) {
	case "", "true", "false":
		return nil, nil
	}
	if depth == 16 {
		return nil, nil
	}
	*budget -= 1
	if *budget < 0 {
		return nil, errors.New("OpenAPI generated JSON body exceeds 4096 schema nodes")
	}
	var schema struct {
		Type                    json.RawMessage
		Example, Default, Const json.RawMessage
		Examples, Enum          []json.RawMessage
		AllOf, OneOf, AnyOf     []json.RawMessage
		Properties              map[string]json.RawMessage
		Items                   json.RawMessage
		ReadOnly                bool
	}
	if err := references.decode(raw, &schema); err != nil {
		return nil, err
	}
	if schema.ReadOnly {
		return nil, nil
	}
	for _, values := range [][]json.RawMessage{{schema.Example}, schema.Examples, {schema.Default, schema.Const}, schema.Enum} {
		for _, value := range values {
			if value != nil {
				return value, nil
			}
		}
	}
	alternatives := schema.OneOf
	if len(alternatives) == 0 {
		alternatives = schema.AnyOf
	}
	var alternative any
	for _, branch := range alternatives {
		value, err := references.schemaValue(branch, depth+1, budget)
		if err != nil {
			return nil, err
		}
		if value != nil {
			alternative = value
			break
		}
	}
	var kind string
	if json.Unmarshal(schema.Type, &kind) != nil {
		var types []string
		if json.Unmarshal(schema.Type, &types) == nil {
			for _, candidate := range types {
				if candidate != "null" {
					kind = candidate
					break
				}
			}
		}
	}
	if kind == "" && (schema.Properties != nil || len(schema.AllOf) != 0) {
		kind = "object"
	}
	if kind != "object" && alternative != nil {
		return alternative, nil
	}
	switch kind {
	case "object":
		object := map[string]any{}
		for _, branch := range schema.AllOf {
			value, err := references.schemaValue(branch, depth+1, budget)
			if err != nil {
				return nil, err
			}
			mergeOpenAPIObject(object, value)
		}
		mergeOpenAPIObject(object, alternative)
		for _, name := range slices.Sorted(maps.Keys(schema.Properties)) {
			if sensitiveKey.MatchString(name) {
				continue
			}
			value, err := references.schemaValue(schema.Properties[name], depth+1, budget)
			if err != nil {
				return nil, err
			}
			if value != nil {
				object[name] = value
			}
		}
		return object, nil
	case "array":
		value, err := references.schemaValue(schema.Items, depth+1, budget)
		if value == nil {
			return []any{}, err
		}
		return []any{value}, err
	case "integer", "number":
		return 0, nil
	case "boolean":
		return false, nil
	case "string":
		return "string", nil
	case "null":
		return json.RawMessage("null"), nil
	default:
		return nil, nil
	}
}

func mergeOpenAPIObject(target map[string]any, value any) {
	if raw, ok := value.(json.RawMessage); ok {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) == nil {
			for key, field := range object {
				if !sensitiveKey.MatchString(key) {
					target[key] = field
				}
			}
		}
	}
	if object, ok := value.(map[string]any); ok {
		maps.Copy(target, object)
	}
}
