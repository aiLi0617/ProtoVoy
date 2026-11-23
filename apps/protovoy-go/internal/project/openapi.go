package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

var openAPIPathParameter = regexp.MustCompile(`\{([A-Za-z0-9_.-]+)\}`)

type openAPIServer struct {
	URL       string
	Variables map[string]struct{ Default string }
}

type openAPIParameter struct {
	Name, In string
	openAPIExamples
}

type openAPIExamples struct {
	Example  json.RawMessage
	Examples map[string]json.RawMessage
	Schema   json.RawMessage
}

type openAPIOperation struct {
	Summary, OperationID string
	Tags                 []string
	Servers              []openAPIServer
	Parameters           []json.RawMessage
	RequestBody          json.RawMessage
}

type openAPIRequestBody struct {
	Content map[string]openAPIExamples
}

func readOpenAPI(data []byte) (Project, error) {
	var source struct {
		OpenAPI string
		Info    struct{ Title string }
		Servers []openAPIServer
		Paths   map[string]json.RawMessage
	}
	if json.Unmarshal(data, &source) != nil || source.Paths == nil ||
		(!strings.HasPrefix(source.OpenAPI, "3.0.") && !strings.HasPrefix(source.OpenAPI, "3.1.")) {
		return Project{}, errors.New("expected an OpenAPI 3.0 or 3.1 JSON document with paths")
	}
	result := Project{Name: strings.TrimSpace(source.Info.Title)}
	references := openAPIReferences{document: data, remaining: 2 * MaxBytes}
	bodyBytes := 0
	if result.Name == "" {
		result.Name = "OpenAPI Import"
	}
	for _, path := range slices.Sorted(maps.Keys(source.Paths)) {
		if !strings.HasPrefix(path, "/") {
			continue
		}
		var item map[string]json.RawMessage
		if err := references.decode(source.Paths[path], &item); err != nil {
			return Project{}, err
		}
		var pathServers []openAPIServer
		if item["servers"] != nil && json.Unmarshal(item["servers"], &pathServers) != nil {
			return Project{}, errors.New("invalid OpenAPI path servers")
		}
		var pathParameters []json.RawMessage
		if item["parameters"] != nil && json.Unmarshal(item["parameters"], &pathParameters) != nil {
			return Project{}, errors.New("invalid OpenAPI path parameters")
		}
		for _, method := range []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"} {
			if item[method] == nil {
				continue
			}
			var operation openAPIOperation
			if json.Unmarshal(item[method], &operation) != nil || bytes.Equal(item[method], []byte("null")) {
				return Project{}, errors.New("invalid OpenAPI operation")
			}
			base := ""
			for _, servers := range [][]openAPIServer{operation.Servers, pathServers, source.Servers} {
				if len(servers) == 0 {
					continue
				}
				server := servers[0]
				base = openAPIPathParameter.ReplaceAllStringFunc(server.URL, func(parameter string) string {
					if value, ok := server.Variables[parameter[1:len(parameter)-1]]; ok {
						return value.Default
					}
					return parameter
				})
				break
			}
			request, err := operation.request(strings.ToUpper(method), base, path, pathParameters, &references)
			if err != nil {
				return Project{}, fmt.Errorf("OpenAPI operation %d: %w", len(result.Requests)+1, err)
			}
			bodyBytes += len(request.Body)
			if bodyBytes > MaxBytes {
				return Project{}, errors.New("OpenAPI request bodies exceed 8 MiB")
			}
			if err := result.Append(request); err != nil {
				return Project{}, err
			}
		}
	}
	if len(result.Requests) == 0 {
		return Project{}, errors.New("OpenAPI document has no HTTP operations")
	}
	return result, nil
}

func (operation openAPIOperation) request(method, base, path string, inherited []json.RawMessage, references *openAPIReferences) (Request, error) {
	if strings.ContainsAny(base, "{}?#") || strings.ContainsAny(path, "?#") {
		return Request{}, errors.New("unsupported OpenAPI server URL or path")
	}
	target := strings.TrimRight(base, "/") + openAPIPathParameter.ReplaceAllStringFunc(path, func(parameter string) string {
		return "{" + parameter + "}"
	})
	if base == "" || strings.HasPrefix(base, "/") && !strings.HasPrefix(base, "//") {
		target = "{{baseUrl}}" + target
	}
	request := Request{Method: method, URL: target, Headers: map[string]string{}, Variables: map[string]string{}}
	request.Name = strings.TrimSpace(operation.Summary)
	if request.Name == "" {
		request.Name = strings.TrimSpace(operation.OperationID)
	}
	if request.Name == "" {
		request.Name = method + " " + path
	}
	if len(operation.Tags) != 0 && strings.TrimSpace(operation.Tags[0]) != "" {
		request.CollectionPath = []string{operation.Tags[0]}
	}
	query := url.Values{}
	parameters := map[[2]string]openAPIParameter{}
	for _, group := range [][]json.RawMessage{inherited, operation.Parameters} {
		for _, raw := range group {
			var parameter openAPIParameter
			if err := references.decode(raw, &parameter); err != nil {
				return Request{}, err
			}
			parameters[[2]string{parameter.In, parameter.Name}] = parameter
		}
	}
	for _, parameter := range parameters {
		if parameter.Name == "" || sensitiveKey.MatchString(parameter.Name) {
			continue
		}
		example, err := parameter.example(references)
		if err != nil {
			return Request{}, err
		}
		value, ok := openAPIScalar(example)
		if !ok {
			continue
		}
		switch parameter.In {
		case "path":
			if strings.Contains(path, "{"+parameter.Name+"}") {
				request.Variables[parameter.Name] = url.PathEscape(value)
			}
		case "query":
			query.Set(parameter.Name, value)
		case "header":
			request.Headers[parameter.Name] = value
		}
	}
	if len(query) != 0 {
		request.URL += "?" + query.Encode()
	}
	var requestBody openAPIRequestBody
	if err := references.decode(operation.RequestBody, &requestBody); err != nil {
		return Request{}, err
	}
	for _, mediaType := range slices.Sorted(maps.Keys(requestBody.Content)) {
		media := requestBody.Content[mediaType]
		if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
			request.Headers["Content-Type"] = mediaType
			body, err := media.example(references)
			if err != nil {
				return Request{}, err
			}
			if body == nil && len(media.Schema) != 0 {
				body, err = references.generateBody(media.Schema)
				if err != nil {
					return Request{}, err
				}
			}
			if body != nil {
				var formatted bytes.Buffer
				if json.Indent(&formatted, body, "", "  ") != nil {
					return Request{}, errors.New("invalid OpenAPI JSON body example")
				}
				request.Body = formatted.String()
			}
			break
		}
	}
	checked := request
	checked.URL = strings.Replace(checked.URL, "{{baseUrl}}", "https://example.invalid", 1)
	if err := validateImportedHTTP(checked); err != nil {
		return Request{}, err
	}
	return request, nil
}

func (examples openAPIExamples) example(references *openAPIReferences) (json.RawMessage, error) {
	if examples.Example != nil {
		return examples.Example, nil
	}
	for _, name := range slices.Sorted(maps.Keys(examples.Examples)) {
		var entry struct{ Value json.RawMessage }
		if err := references.decode(examples.Examples[name], &entry); err != nil {
			return nil, err
		}
		if entry.Value != nil {
			return entry.Value, nil
		}
	}
	switch string(examples.Schema) {
	case "", "true", "false":
		return nil, nil
	}
	var schema struct {
		Example, Default json.RawMessage
		Examples         []json.RawMessage
	}
	if err := references.decode(examples.Schema, &schema); err != nil {
		return nil, err
	}
	if schema.Example != nil {
		return schema.Example, nil
	}
	if len(schema.Examples) != 0 {
		return schema.Examples[0], nil
	}
	return schema.Default, nil
}

func openAPIScalar(raw json.RawMessage) (string, bool) {
	var text string
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}
	if raw[0] == '"' {
		err := json.Unmarshal(raw, &text)
		return text, err == nil
	}
	if raw[0] == '[' || raw[0] == '{' {
		return "", false
	}
	return string(raw), true
}
