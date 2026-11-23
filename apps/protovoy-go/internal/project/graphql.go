package project

import (
	"encoding/json"
	"errors"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

type graphQLFile struct {
	Name       string             `json:"name"`
	ProtocolID string             `json:"protocolId"`
	Target     string             `json:"target"`
	Variables  map[string]string  `json:"variables,omitempty"`
	TimeoutMS  int                `json:"timeoutMs,omitempty"`
	Payload    graphQLFilePayload `json:"payload"`
}

type graphQLFilePayload struct {
	Type          string          `json:"type"`
	Query         string          `json:"query"`
	Variables     json.RawMessage `json:"variables"`
	OperationName string          `json:"operationName,omitempty"`
	Subscription  bool            `json:"subscription,omitempty"`
	Headers       [][2]string     `json:"headers,omitempty"`
}

func readGraphQLRequest(request Request, timeout int, data []byte) (Project, error) {
	var payload graphQLFilePayload
	if !decodeLegacy(data, &payload) || payload.Type != "graphql" || strings.TrimSpace(payload.Query) == "" {
		return Project{}, errors.New("invalid GraphQL request file")
	}
	variables := payload.Variables
	if len(variables) == 0 || string(variables) == "null" {
		variables = json.RawMessage("{}")
	}
	if !isJSONObject(variables) {
		return Project{}, errors.New("GraphQL variables must be a JSON object")
	}
	headers, err := uniqueHeaderMap(payload.Headers)
	if err != nil {
		return Project{}, err
	}
	if request.Name == "" {
		request.Name = "Imported GraphQL request"
	}
	subscription := payload.Subscription || startsWithSubscription(payload.Query)
	if subscription && timeout == execution.DefaultRequestTimeoutMS {
		timeout = 0
	}
	document := execution.GraphQLDocument{
		Query: payload.Query, Variables: string(variables), OperationName: payload.OperationName,
		Subscription: subscription, TimeoutMS: timeout,
	}
	if err := ValidateGraphQLTimeout(document); err != nil {
		return Project{}, err
	}
	request.Headers, request.GraphQL = headers, &document
	return Project{Name: request.Name, Requests: []Request{request}}, nil
}

func EncodeGraphQL(name, target string, headers [][2]string, document execution.GraphQLDocument, values map[string]string) ([]byte, error) {
	if strings.TrimSpace(target) == "" || strings.TrimSpace(document.Query) == "" {
		return nil, errors.New("enter a URL and query before exporting GraphQL")
	}
	headerMap, err := uniqueHeaderMap(headers)
	if err != nil {
		return nil, err
	}
	credentials := Request{URL: target, GraphQL: &document, Variables: values, Headers: headerMap}
	if err := ValidateGraphQLTimeout(document); err != nil {
		return nil, err
	}
	if credentials.containsSensitiveData() {
		return nil, errors.New("remove possible credentials before exporting")
	}
	variables := strings.TrimSpace(document.Variables)
	if variables == "" {
		variables = "{}"
	}
	if !isJSONObject([]byte(variables)) {
		return nil, errors.New("GraphQL variables must be a JSON object")
	}
	file := graphQLFile{Name: name, ProtocolID: "graphql", Target: target, Variables: values, TimeoutMS: document.TimeoutMS}
	file.Payload.Type = "graphql"
	file.Payload.Query = document.Query
	file.Payload.Variables = json.RawMessage(variables)
	file.Payload.OperationName = document.OperationName
	file.Payload.Subscription = document.Subscription
	file.Payload.Headers = headers
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil || len(data) > MaxBytes {
		return nil, errors.New("could not encode GraphQL request within 8 MiB")
	}
	return data, nil
}

func ValidateGraphQLTimeout(document execution.GraphQLDocument) error {
	if !execution.ValidRequestTimeout(document.TimeoutMS) ||
		document.TimeoutMS != 0 && (document.Subscription || startsWithSubscription(document.Query)) {
		return errors.New("custom timeout requires a GraphQL HTTP operation and a value up to five minutes")
	}
	return nil
}

func startsWithSubscription(query string) bool {
	for {
		query = strings.TrimLeft(query, " \t\n\r,\uFEFF")
		if !strings.HasPrefix(query, "#") {
			break
		}
		end := strings.IndexAny(query, "\r\n")
		if end == -1 {
			return false
		}
		query = query[end:]
	}
	rest, found := strings.CutPrefix(query, "subscription")
	return found && (rest == "" || strings.ContainsRune(" \t\r\n,{(@#", rune(rest[0])))
}

func isJSONObject(data []byte) bool {
	var value map[string]json.RawMessage
	return json.Unmarshal(data, &value) == nil && value != nil
}

func uniqueHeaderMap(headers [][2]string) (map[string]string, error) {
	values := make(map[string]string, len(headers))
	seen := make(map[string]bool, len(headers))
	for _, header := range headers {
		key := strings.ToLower(header[0])
		if seen[key] || !validHeader(header[0], header[1]) {
			return nil, errors.New("request file requires valid, unique header names")
		}
		seen[key] = true
		values[header[0]] = header[1]
	}
	return values, nil
}
