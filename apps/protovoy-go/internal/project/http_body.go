package project

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

func ParseHTTPMultipart(input string) ([]execution.MultipartPart, error) {
	var parts []execution.MultipartPart
	if !decodeLegacy([]byte(input), &parts) || len(parts) == 0 || len(parts) > 128 {
		return nil, errors.New("enter a JSON array of 1 to 128 multipart parts")
	}
	for _, part := range parts {
		if strings.TrimSpace(part.Name) == "" {
			return nil, errors.New("each multipart part requires a name")
		}
	}
	return parts, nil
}

func sensitiveMultipart(parts []execution.MultipartPart) bool {
	for _, part := range parts {
		if sensitiveKey.MatchString(part.Name) || sensitiveHTTPBody(part.Value, part.Base64) {
			return true
		}
	}
	return false
}

func sensitiveHTTPBody(body string, encoded bool) bool {
	if sensitiveBodySource(body) {
		return true
	}
	if !encoded {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(body)
	return err == nil && sensitiveBodySource(string(decoded))
}

func (request Request) ValidateHTTPBody() error {
	if request.BodyEncoding != "" && request.BodyEncoding != "text" && request.BodyEncoding != "base64" {
		return errors.New("invalid HTTP body encoding")
	}
	if request.BodySource != "" {
		if len(request.BodySource) > 1<<20 || !json.Valid([]byte(request.BodySource)) {
			return errors.New("HTTP body source must contain valid JSON within 1 MiB")
		}
		if sensitiveBodySource(request.BodySource) {
			return errors.New("remove possible credentials from the HTTP body source")
		}
	}
	if len(request.Multipart) == 0 {
		return nil
	}
	if request.Body != "" || request.BodyEncoding == "base64" {
		return errors.New("multipart requests cannot include a separate HTTP body or Base64 body encoding")
	}
	if len(request.Multipart) > 128 {
		return errors.New("multipart requests support up to 128 parts")
	}
	for _, part := range request.Multipart {
		if strings.TrimSpace(part.Name) == "" {
			return errors.New("each multipart part requires a name")
		}
	}
	for name := range request.Headers {
		if strings.EqualFold(name, "Content-Type") {
			return errors.New("remove the Content-Type header for multipart requests")
		}
	}
	if sensitiveMultipart(request.Multipart) {
		return errors.New("remove possible credentials from multipart parts")
	}
	return nil
}

func sensitiveBodySource(source string) bool {
	return ContainsRecognizableBodyCredentials(source)
}

// ContainsRecognizableBodyCredentials applies the project's text credential heuristics.
func ContainsRecognizableBodyCredentials(body string) bool {
	return sensitiveBody.MatchString(body) || hasPrivateKey(body)
}
