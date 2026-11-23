package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

type harRequest struct {
	Method   string
	URL      string
	Headers  []struct{ Name, Value string }
	BodySize int
	PostData *struct {
		MIMEType string
		Text     *string
		Encoding string
		Params   []struct{ Name, Value, FileName string }
	}
}

func readHAR(data []byte) (Project, error) {
	var document struct {
		Log struct {
			Version string
			Entries []struct{ Request harRequest }
		}
	}
	if json.Unmarshal(data, &document) != nil || document.Log.Version != "1.2" || document.Log.Entries == nil {
		return Project{}, errors.New("expected a HAR 1.2 file")
	}
	if len(document.Log.Entries) > maxRequests {
		return Project{}, errors.New("HAR exceeds 10,000 requests")
	}
	result := Project{Name: "HAR Import", Requests: make([]Request, 0, len(document.Log.Entries))}
	for index, entry := range document.Log.Entries {
		request, err := entry.Request.portable()
		if err != nil {
			return Project{}, fmt.Errorf("HAR request %d: %w", index+1, err)
		}
		result.Requests = append(result.Requests, request)
	}
	return result, nil
}

func (source harRequest) portable() (Request, error) {
	httpRequest, err := http.NewRequest(source.Method, source.URL, nil)
	if err != nil || source.Method == "" || httpRequest.URL.Hostname() == "" || httpRequest.URL.User != nil ||
		(httpRequest.URL.Scheme != "http" && httpRequest.URL.Scheme != "https") {
		return Request{}, errors.New("invalid HTTP or HTTPS method or URL")
	}
	request := Request{Method: source.Method, URL: source.URL, Name: httpRequest.URL.EscapedPath(), Headers: map[string]string{}}
	if request.Name == "" {
		request.Name = "/"
	}
	for _, header := range source.Headers {
		name := http.CanonicalHeaderKey(header.Name)
		if sensitiveKey.MatchString(name) || strings.HasPrefix(name, ":") {
			continue
		}
		switch name {
		case "Accept-Encoding", "Connection", "Content-Length", "Host", "Keep-Alive", "Proxy-Connection", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
			continue
		}
		if !validHeader(name, header.Value) {
			return Request{}, errors.New("invalid HTTP header")
		}
		if _, exists := request.Headers[name]; exists {
			return Request{}, errors.New("repeated HTTP headers cannot be represented in a project")
		}
		request.Headers[name] = header.Value
	}
	if source.PostData != nil {
		post := source.PostData
		if post.Encoding != "" || post.Text != nil && len(post.Params) != 0 {
			return Request{}, errors.New("unsupported request body representation")
		}
		if post.Text != nil {
			request.Body = *post.Text
		}
		if _, exists := request.Headers["Content-Type"]; !exists && post.MIMEType != "" {
			request.Headers["Content-Type"] = post.MIMEType
		}
		if len(post.Params) != 0 {
			mediaType, _, _ := mime.ParseMediaType(request.Headers["Content-Type"])
			if mediaType != "application/x-www-form-urlencoded" {
				return Request{}, errors.New("only URL-encoded form parameters can be reconstructed")
			}
			fields := make([]string, 0, len(post.Params))
			for _, field := range post.Params {
				if field.FileName != "" {
					return Request{}, errors.New("HAR file references cannot be imported")
				}
				fields = append(fields, url.QueryEscape(field.Name)+"="+url.QueryEscape(field.Value))
			}
			request.Body = strings.Join(fields, "&")
		}
	}
	if source.BodySize > 0 && request.Body == "" {
		return Request{}, errors.New("captured request body is missing")
	}
	if err := validateImportedHTTP(request); err != nil {
		return Request{}, err
	}
	return request, nil
}

func validateImportedHTTP(request Request) error {
	parsed, err := http.NewRequest(request.Method, request.URL, nil)
	if err != nil || parsed.URL.Hostname() == "" || parsed.URL.User != nil ||
		(parsed.URL.Scheme != "http" && parsed.URL.Scheme != "https") {
		return errors.New("invalid HTTP or HTTPS method or URL")
	}
	if err := validatePortableRequest(&request, true); err != nil {
		return err
	}
	mediaType, _, _ := mime.ParseMediaType(request.Headers["Content-Type"])
	encoded := []string{parsed.URL.RawQuery}
	if mediaType == "application/x-www-form-urlencoded" {
		encoded = append(encoded, request.Body)
	}
	for _, part := range encoded {
		values, err := url.ParseQuery(part)
		if err != nil {
			return errors.New("invalid query or form encoding")
		}
		for key := range values {
			if sensitiveKey.MatchString(key) {
				return errors.New("remove possible credentials from the URL or body")
			}
		}
	}
	if request.containsSensitiveData() || hasPrivateKey(request.Body) {
		return errors.New("remove possible credentials from the request")
	}
	return nil
}
