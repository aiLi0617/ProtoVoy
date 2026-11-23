package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type postmanItem struct {
	Name    string
	Item    []postmanItem
	Request json.RawMessage
	Event   []postmanEvent
}

type postmanEvent struct {
	Disabled bool
	Script   json.RawMessage
}

func hasPostmanScripts(events []postmanEvent) bool {
	for _, event := range events {
		if !event.Disabled && len(event.Script) != 0 && string(event.Script) != "null" {
			return true
		}
	}
	return false
}

type postmanField struct {
	Key, Value string
	Disabled   bool
}

type postmanRequest struct {
	Method string
	URL    json.RawMessage
	Header []postmanField
	Body   *struct {
		Mode, Raw  string
		Disabled   bool
		URLEncoded []postmanField
		Options    struct {
			Raw struct{ Language string }
		}
	}
}

func readPostman(data []byte) (Project, error) {
	var source struct {
		Info     struct{ Name string }
		Item     []postmanItem
		Variable []postmanField
		Event    []postmanEvent
	}
	if json.Unmarshal(data, &source) != nil || source.Item == nil {
		return Project{}, errors.New("invalid Postman collection")
	}
	if hasPostmanScripts(source.Event) {
		return Project{}, errors.New("Postman collection contains scripts that Viva cannot execute")
	}
	result := Project{Name: strings.TrimSpace(source.Info.Name), CollectionVariables: map[string]string{}}
	if result.Name == "" {
		result.Name = "Postman Import"
	}
	for _, field := range source.Variable {
		if field.Disabled || field.Key == "" {
			continue
		}
		if sensitiveKey.MatchString(field.Key) || hasPrivateKey(field.Value) {
			return Project{}, errors.New("remove possible credentials from Postman variables before importing")
		}
		result.CollectionVariables[field.Key] = field.Value
	}
	nodes := 0
	if err := appendPostmanItems(&result, source.Item, nil, &nodes); err != nil {
		return Project{}, err
	}
	return result, nil
}

func appendPostmanItems(result *Project, items []postmanItem, path []string, nodes *int) error {
	if len(path) > maxCollectionDepth {
		return errors.New("Postman folder nesting exceeds 64 levels")
	}
	for _, item := range items {
		*nodes += 1
		if *nodes > maxRequests {
			return errors.New("Postman collection exceeds 10,000 items")
		}
		if hasPostmanScripts(item.Event) {
			return fmt.Errorf("Postman item %d contains scripts that Viva cannot execute", *nodes)
		}
		name := strings.TrimSpace(item.Name)
		if item.Item != nil {
			if len(item.Request) != 0 {
				return errors.New("Postman item cannot be both a folder and a request")
			}
			if name == "" {
				name = "Folder"
			}
			child := append(append([]string(nil), path...), name)
			if len(item.Item) == 0 {
				result.EmptyFolders = append(result.EmptyFolders, child)
			}
			if err := appendPostmanItems(result, item.Item, child, nodes); err != nil {
				return err
			}
			continue
		}
		request, err := readPostmanRequest(item.Request, result.CollectionVariables)
		if err != nil {
			return fmt.Errorf("Postman item %d: %w", *nodes, err)
		}
		if name == "" {
			name = "Imported request"
		}
		request.Name, request.CollectionPath = name, path
		result.Requests = append(result.Requests, request)
	}
	return nil
}

func readPostmanRequest(data json.RawMessage, variables map[string]string) (Request, error) {
	var source postmanRequest
	if len(data) != 0 && data[0] == '"' {
		source.URL = data
	} else if json.Unmarshal(data, &source) != nil {
		return Request{}, errors.New("invalid HTTP request")
	}
	request := Request{Method: strings.ToUpper(source.Method), Headers: map[string]string{}}
	if request.Method == "" {
		request.Method = http.MethodGet
	}
	if json.Unmarshal(source.URL, &request.URL) != nil {
		var target struct{ Raw string }
		if json.Unmarshal(source.URL, &target) != nil {
			return Request{}, errors.New("expected a Postman URL string or raw URL")
		}
		request.URL = target.Raw
	}
	for _, field := range source.Header {
		name := http.CanonicalHeaderKey(field.Key)
		if field.Disabled || sensitiveKey.MatchString(name) {
			continue
		}
		if !validHeader(name, field.Value) {
			return Request{}, errors.New("invalid HTTP header")
		}
		if _, exists := request.Headers[name]; exists {
			return Request{}, errors.New("repeated HTTP headers cannot be represented in a project")
		}
		request.Headers[name] = field.Value
	}
	if body := source.Body; body != nil && !body.Disabled {
		var contentType string
		switch body.Mode {
		case "", "raw":
			request.Body = body.Raw
			contentType = map[string]string{"json": "application/json", "text": "text/plain", "xml": "application/xml",
				"html": "text/html", "javascript": "application/javascript"}[body.Options.Raw.Language]
		case "urlencoded":
			contentType = "application/x-www-form-urlencoded"
			fields := make([]string, 0, len(body.URLEncoded))
			for _, field := range body.URLEncoded {
				if field.Disabled {
					continue
				}
				if strings.Contains(field.Key+field.Value, "{{") {
					return Request{}, errors.New("Postman URL-encoded form variables require manual conversion")
				}
				fields = append(fields, url.QueryEscape(field.Key)+"="+url.QueryEscape(field.Value))
			}
			request.Body = strings.Join(fields, "&")
		default:
			return Request{}, errors.New("unsupported Postman body mode")
		}
		if _, exists := request.Headers["Content-Type"]; !exists && contentType != "" {
			request.Headers["Content-Type"] = contentType
		}
	}
	input, err := request.PrepareCollectionHTTP(variables)
	if err != nil {
		return Request{}, err
	}
	resolved := Request{Method: input.Method, URL: input.Target, Body: input.Body, Headers: map[string]string{}}
	for _, header := range input.Headers {
		resolved.Headers[http.CanonicalHeaderKey(header[0])] = header[1]
	}
	if err := validateImportedHTTP(resolved); err != nil {
		return Request{}, err
	}
	return request, nil
}
