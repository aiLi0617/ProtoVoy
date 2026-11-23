package execution

import (
	"encoding/json"
	"net/url"
	"strings"
)

// HTTPDefault is a legacy environment parameter applied after request templates.
type HTTPDefault struct {
	Location      string `json:"location"`
	Name          string `json:"name"`
	Value         string `json:"value"`
	Enabled       *bool  `json:"enabled,omitempty"`
	ParameterType string `json:"parameterType,omitempty"`
	Required      bool   `json:"required,omitempty"`
	Description   string `json:"description,omitempty"`
}

func (payload *HTTPPayload) applyHTTPDefault(target, location, name, value string) string {
	switch location {
	case "header":
		payload.Headers = defaultHeader(payload.Headers, name, value)
	case "query":
		if endpoint, err := url.Parse(target); err == nil && endpoint.Host != "" && !endpoint.Query().Has(name) {
			if endpoint.RawQuery != "" {
				endpoint.RawQuery += "&"
			}
			endpoint.RawQuery += url.QueryEscape(name) + "=" + url.QueryEscape(value)
			target = endpoint.String()
		}
	case "body":
		payload.applyBodyDefault(name, value)
	}
	return target
}

func (payload *HTTPPayload) applyBodyDefault(name, value string) {
	if len(payload.Multipart) != 0 {
		for _, part := range payload.Multipart {
			if part.Name == name {
				return
			}
		}
		payload.Multipart = append(payload.Multipart, MultipartPart{Name: name, Value: value})
		return
	}
	if payload.BodyEncoding == "base64" {
		return
	}
	var contentType string
	for _, header := range payload.Headers {
		if strings.EqualFold(header[0], "Content-Type") {
			contentType = strings.ToLower(header[1])
			break
		}
	}
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		values, err := url.ParseQuery(payload.Body)
		if err != nil || values.Has(name) {
			return
		}
		if payload.Body != "" {
			payload.Body += "&"
		}
		payload.Body += url.QueryEscape(name) + "=" + url.QueryEscape(value)
		return
	}
	body := strings.TrimSpace(payload.Body)
	if body != "" && contentType != "" && !strings.Contains(contentType, "application/json") {
		return
	}
	object := make(map[string]json.RawMessage)
	if body != "" && (json.Unmarshal([]byte(body), &object) != nil || object == nil) {
		return
	}
	if _, exists := object[name]; exists {
		return
	}
	object[name], _ = json.Marshal(value)
	data, _ := json.Marshal(object)
	payload.Body = string(data)
	payload.Headers = defaultHeader(payload.Headers, "Content-Type", "application/json")
}
