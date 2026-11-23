package execution

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
)

// HTTPSourceKind identifies the source formats stored by the legacy HTTP editor.
func HTTPSourceKind(encoding, source string) string {
	if source == "" {
		return ""
	}
	if encoding == "base64" {
		return "messagepack"
	}
	var header struct{ Type string }
	if json.Unmarshal([]byte(source), &header) == nil && (header.Type == "graphql" || header.Type == "jsonrpc") {
		return header.Type
	}
	return ""
}

func EncodeHTTPSource(kind, source string) (string, error) {
	if len(source) > 1<<20 || !json.Valid([]byte(source)) {
		return "", errors.New("enter valid HTTP body source JSON within 1 MiB")
	}
	var body []byte
	var err error
	switch kind {
	case "graphql":
		var input struct {
			Type, Query, OperationName string
			Variables                  json.RawMessage
		}
		if json.Unmarshal([]byte(source), &input) != nil || input.Type != kind || strings.TrimSpace(input.Query) == "" {
			return "", errors.New("GraphQL source requires type graphql and a query")
		}
		body, err = json.Marshal(struct {
			Query         string          `json:"query"`
			Variables     json.RawMessage `json:"variables,omitempty"`
			OperationName string          `json:"operationName,omitempty"`
		}{input.Query, input.Variables, strings.TrimSpace(input.OperationName)})
	case "jsonrpc":
		var input struct {
			Type, Method string
			Params, ID   json.RawMessage
		}
		if json.Unmarshal([]byte(source), &input) != nil || input.Type != kind || strings.TrimSpace(input.Method) == "" {
			return "", errors.New("JSON-RPC source requires type jsonrpc and a method")
		}
		body, err = json.Marshal(struct {
			Version string          `json:"jsonrpc"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params,omitempty"`
			ID      json.RawMessage `json:"id,omitempty"`
		}{"2.0", input.Method, input.Params, input.ID})
	case "messagepack":
		decoder := json.NewDecoder(strings.NewReader(source))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) != nil {
			return "", errors.New("invalid MessagePack source JSON")
		}
		value, err = messagePackValue(value, 0)
		if err != nil {
			return "", err
		}
		var out bytes.Buffer
		encoder := msgpack.NewEncoder(&out).SetSortMapKeys(true)
		encoder.UseCompactInts(true)
		if err := encoder.Encode(value); err != nil || out.Len() > 1<<20 {
			return "", errors.New("could not encode MessagePack body within 1 MiB")
		}
		return base64.StdEncoding.EncodeToString(out.Bytes()), nil
	default:
		return "", errors.New("unsupported HTTP body source format")
	}
	if err != nil || len(body) > 1<<20 {
		return "", errors.New("could not encode HTTP body within 1 MiB")
	}
	return string(body), nil
}

func messagePackValue(value any, depth int) (any, error) {
	if depth > 128 {
		return nil, errors.New("MessagePack source exceeds 128 levels of nesting")
	}
	switch value := value.(type) {
	case json.Number:
		if number, err := value.Int64(); err == nil {
			return number, nil
		}
		if number, err := strconv.ParseUint(value.String(), 10, 64); err == nil {
			return number, nil
		}
		if strings.ContainsAny(value.String(), ".eE") {
			if number, err := value.Float64(); err == nil {
				return number, nil
			}
		}
		return nil, errors.New("MessagePack source contains a number outside its supported range")
	case map[string]any:
		for key, child := range value {
			converted, err := messagePackValue(child, depth+1)
			if err != nil {
				return nil, err
			}
			value[key] = converted
		}
	case []any:
		for i, child := range value {
			converted, err := messagePackValue(child, depth+1)
			if err != nil {
				return nil, err
			}
			value[i] = converted
		}
	}
	return value, nil
}
