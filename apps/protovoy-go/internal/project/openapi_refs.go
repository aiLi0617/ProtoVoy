package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

type openAPIReferences struct {
	document  []byte
	root      any
	cache     map[string]json.RawMessage
	remaining int
}

var jsonPointerUnescape = strings.NewReplacer("~1", "/", "~0", "~")

func (resolver *openAPIReferences) decode(raw json.RawMessage, output any) error {
	if raw == nil {
		return nil
	}
	seen := map[string]bool{}
	for {
		var reference struct {
			Ref *string `json:"$ref"`
		}
		if json.Unmarshal(raw, &reference) != nil || bytes.Equal(raw, []byte("null")) {
			return errors.New("expected an OpenAPI object")
		}
		if reference.Ref == nil {
			if json.Unmarshal(raw, output) != nil {
				return errors.New("invalid OpenAPI object")
			}
			return nil
		}
		pointer, err := url.PathUnescape(strings.TrimPrefix(*reference.Ref, "#"))
		if err != nil || !strings.HasPrefix(*reference.Ref, "#") || !strings.HasPrefix(pointer, "/") {
			return errors.New("OpenAPI references must use a local JSON Pointer")
		}
		if seen[pointer] || len(seen) == maxCollectionDepth {
			return errors.New("OpenAPI reference cycle or excessive reference depth")
		}
		seen[pointer] = true
		raw, err = resolver.lookup(pointer)
		if err != nil {
			return err
		}
		resolver.remaining -= len(raw)
		if resolver.remaining < 0 {
			return errors.New("OpenAPI reference expansion exceeds 16 MiB")
		}
	}
}

func (resolver *openAPIReferences) lookup(pointer string) (json.RawMessage, error) {
	if raw, ok := resolver.cache[pointer]; ok {
		return raw, nil
	}
	if resolver.root == nil {
		decoder := json.NewDecoder(bytes.NewReader(resolver.document))
		decoder.UseNumber()
		if decoder.Decode(&resolver.root) != nil {
			return nil, errors.New("invalid OpenAPI reference document")
		}
		resolver.cache = make(map[string]json.RawMessage)
	}
	current := resolver.root
	for _, token := range strings.Split(pointer[1:], "/") {
		for index := 0; index < len(token); index++ {
			if token[index] == '~' && (index+1 == len(token) || token[index+1] != '0' && token[index+1] != '1') {
				return nil, errors.New("invalid OpenAPI reference pointer escape")
			}
		}
		key := jsonPointerUnescape.Replace(token)
		switch value := current.(type) {
		case map[string]any:
			current = value[key]
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(value) || strconv.Itoa(index) != key {
				return nil, errors.New("invalid OpenAPI reference array index")
			}
			current = value[index]
		default:
			return nil, errors.New("OpenAPI reference target does not exist")
		}
	}
	if current == nil {
		return nil, errors.New("OpenAPI reference target does not exist")
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return nil, errors.New("invalid OpenAPI reference target")
	}
	resolver.cache[pointer] = raw
	return raw, nil
}
