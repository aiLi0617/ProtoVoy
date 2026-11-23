package execution

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// GraphQLDocument holds the editable document before variable resolution.
type GraphQLDocument struct {
	Query         string `json:"query"`
	Variables     string `json:"variables,omitempty"`
	OperationName string `json:"operationName,omitempty"`
	Subscription  bool   `json:"subscription,omitempty"`
	TimeoutMS     int    `json:"timeoutMs,omitempty"`
}

// Prepare builds a GraphQL over HTTP POST. Template substitutions inside
// variables happen after JSON parsing, so inserted quotes stay string data.
func (document GraphQLDocument) Prepare(target string, headers [][2]string, scopes ...map[string]string) (string, HTTPPayload, error) {
	if !ValidRequestTimeout(document.TimeoutMS) {
		return "", HTTPPayload{}, errors.New("invalid GraphQL HTTP timeout")
	}
	target, payload, err := document.resolve(target, headers, scopes)
	if err != nil {
		return "", HTTPPayload{}, err
	}
	payload.Method = http.MethodPost
	payload.TimeoutMS = document.TimeoutMS
	payload.Headers = defaultHeader(payload.Headers, "Content-Type", "application/json")
	payload.Headers = defaultHeader(payload.Headers, "Accept", "application/graphql-response+json, application/json")
	return target, payload, nil
}

func (document GraphQLDocument) resolve(target string, headers [][2]string, scopes []map[string]string) (string, HTTPPayload, error) {
	const maxSize = 8 << 20
	if len(document.Query)+len(document.Variables)+len(document.OperationName) > maxSize {
		return "", HTTPPayload{}, errors.New("GraphQL document exceeds 8 MiB")
	}
	variables := map[string]any{}
	if strings.TrimSpace(document.Variables) != "" {
		decoder := json.NewDecoder(strings.NewReader(document.Variables))
		decoder.UseNumber()
		if err := decoder.Decode(&variables); err != nil || variables == nil || decoder.Decode(new(any)) != io.EOF {
			return "", HTTPPayload{}, errors.New("GraphQL variables must be a single JSON object")
		}
	}
	resolver := newVariableResolver(scopes)
	target = resolver.text(target)
	headers = resolver.headers(headers)
	query := resolver.text(document.Query)
	operation := resolver.text(document.OperationName)
	variables, _ = resolver.jsonValue(variables, 0).(map[string]any)
	if resolver.err != nil {
		return "", HTTPPayload{}, resolver.err
	}
	if strings.TrimSpace(query) == "" {
		return "", HTTPPayload{}, errors.New("enter a GraphQL document")
	}
	body, err := json.Marshal(struct {
		Query         string         `json:"query"`
		Variables     map[string]any `json:"variables"`
		OperationName string         `json:"operationName,omitempty"`
	}{query, variables, strings.TrimSpace(operation)})
	if err != nil || len(body) > maxSize {
		return "", HTTPPayload{}, errors.New("could not encode GraphQL document within 8 MiB")
	}
	return target, HTTPPayload{Headers: headers, Body: string(body)}, nil
}

func (resolver *variableResolver) jsonValue(value any, depth int) any {
	if resolver.err != nil {
		return nil
	}
	if depth > 128 {
		resolver.err = errors.New("GraphQL variables exceed 128 levels of nesting")
		return nil
	}
	switch value := value.(type) {
	case string:
		return resolver.text(value)
	case []any:
		for index, item := range value {
			value[index] = resolver.jsonValue(item, depth+1)
		}
	case map[string]any:
		resolved := make(map[string]any, len(value))
		for key, item := range value {
			key = resolver.text(key)
			if resolver.err != nil {
				break
			}
			if _, exists := resolved[key]; exists {
				resolver.err = errors.New("GraphQL variable names collide after substitution")
				break
			}
			resolved[key] = resolver.jsonValue(item, depth+1)
		}
		return resolved
	}
	return value
}

func defaultHeader(headers [][2]string, name, value string) [][2]string {
	for _, header := range headers {
		if strings.EqualFold(header[0], name) {
			return headers
		}
	}
	return append(headers, [2]string{name, value})
}

func GraphQLErrorCount(body []byte) int {
	var response struct {
		Errors []json.RawMessage `json:"errors"`
	}
	if json.Unmarshal(body, &response) != nil {
		return 0
	}
	return len(response.Errors)
}
