package project

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"protovoy.dev/protovoy/internal/execution"
)

// CurlSnippet renders a saved HTTP-based request for a POSIX shell.
func (request Request) CurlSnippet(variables map[string]string) (string, error) {
	if request.HTTPSecretReference != "" || request.HTTPSecretHeader != "" {
		return "", errors.New("cURL export does not support Agent credential bindings")
	}
	if request.HasScripts() {
		return "", errors.New("cURL export does not support request scripts")
	}
	if request.containsSensitiveData() || sensitiveVariables(variables) {
		return "", errors.New("remove possible credentials before generating a cURL command")
	}
	input, err := request.PrepareCollectionHTTP(variables)
	if err != nil {
		return "", err
	}
	httpRequest, err := http.NewRequest(input.Method, input.Target, nil)
	if err != nil || httpRequest.URL.Hostname() == "" || httpRequest.URL.User != nil ||
		(httpRequest.URL.Scheme != "http" && httpRequest.URL.Scheme != "https") {
		return "", errors.New("enter a valid HTTP or HTTPS request")
	}
	resolved := Request{URL: input.Target, Body: input.Body, Headers: map[string]string{}}
	hasContentType := false
	for _, header := range input.Headers {
		if strings.ContainsAny(header[0]+header[1], "\r\n\x00") {
			return "", errors.New("cURL command generation requires single-line HTTP headers")
		}
		if strings.EqualFold(header[0], "Content-Length") || strings.EqualFold(header[0], "Transfer-Encoding") {
			return "", errors.New("remove Content-Length and Transfer-Encoding before generating a cURL command")
		}
		hasContentType = hasContentType || strings.EqualFold(header[0], "Content-Type")
		resolved.Headers[header[0]] = header[1]
	}
	if resolved.containsSensitiveData() || hasPrivateKey(input.Body) {
		return "", errors.New("remove possible credentials before generating a cURL command")
	}
	if strings.ContainsRune(input.Body, 0) || !utf8.ValidString(input.Body) {
		return "", errors.New("cURL command generation requires a UTF-8 request body without NUL bytes")
	}
	timeout := input.Timeout
	if timeout <= 0 {
		timeout = execution.RequestTimeout(0)
	}
	args := []string{"curl", "--disable", "--globoff", "--path-as-is", "--proto", "'=http,https'",
		"--max-time", strconv.FormatFloat(timeout.Seconds(), 'f', -1, 64)}
	if input.FollowRedirects {
		return "", errors.New("cURL command generation requires redirects to be disabled")
	}
	if httpRequest.Method == http.MethodHead && input.Body == "" {
		args = append(args, "--head")
	} else {
		args = append(args, "--request", quoteShell(httpRequest.Method))
	}
	for _, header := range input.Headers {
		value := header[0] + ": " + header[1]
		if header[1] == "" {
			value = header[0] + ";"
		}
		args = append(args, "--header", quoteShell(value))
	}
	if input.Body != "" {
		if !hasContentType {
			args = append(args, "--header", "'Content-Type:'")
		}
		args = append(args, "--data-raw", quoteShell(input.Body))
	}
	args = append(args, "--url", quoteShell(input.Target))
	return strings.Join(args, " "), nil
}

func quoteShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
