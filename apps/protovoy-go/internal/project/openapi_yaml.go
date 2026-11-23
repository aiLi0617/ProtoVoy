package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"gopkg.in/yaml.v3"
)

func readOpenAPIYAML(data []byte) (Project, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if decoder.Decode(&document) != nil || len(document.Content) != 1 ||
		document.Content[0].Kind != yaml.MappingNode {
		return Project{}, errors.New("expected a supported project JSON file or OpenAPI YAML document")
	}
	var extra yaml.Node
	if decoder.Decode(&extra) != io.EOF {
		return Project{}, errors.New("OpenAPI YAML must contain exactly one document")
	}
	budget := 100_000
	value, err := openAPIYAMLValue(document.Content[0], 0, &budget)
	if err != nil {
		return Project{}, err
	}
	converted, err := json.Marshal(value)
	if err != nil || len(converted) > MaxBytes {
		return Project{}, errors.New("OpenAPI YAML must convert to JSON within 8 MiB")
	}
	return readOpenAPI(converted)
}

func openAPIYAMLValue(node *yaml.Node, depth int, budget *int) (any, error) {
	*budget -= 1
	if depth > 128 || *budget < 0 {
		return nil, errors.New("OpenAPI YAML exceeds the nesting or node limit")
	}
	switch node.Kind {
	case yaml.MappingNode:
		object := make(map[string]any, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag == "!!merge" {
				return nil, errors.New("OpenAPI YAML requires scalar keys without merge directives")
			}
			if _, exists := object[key.Value]; exists {
				return nil, errors.New("OpenAPI YAML contains a duplicate key")
			}
			value, err := openAPIYAMLValue(node.Content[i+1], depth+1, budget)
			if err != nil {
				return nil, err
			}
			object[key.Value] = value
		}
		return object, nil
	case yaml.SequenceNode:
		array := make([]any, 0, len(node.Content))
		for _, item := range node.Content {
			value, err := openAPIYAMLValue(item, depth+1, budget)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		return array, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str", "!!timestamp":
			return node.Value, nil
		case "!!null", "!!bool", "!!int", "!!float":
			var value any
			if node.Decode(&value) == nil {
				return value, nil
			}
		}
	}
	return nil, errors.New("OpenAPI YAML contains an unsupported value or alias")
}
