package project

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxCurlImportBytes = 1 << 20

// ParseCurl parses a single literal HTTP request without invoking a shell.
func ParseCurl(source string) (Request, error) {
	if len(source) > MaxCurlImportBytes || !utf8.ValidString(source) || strings.ContainsRune(source, 0) {
		return Request{}, errors.New("cURL input must be UTF-8 text within 1 MiB")
	}
	words, err := curlWords(strings.TrimSpace(source))
	if err != nil {
		return Request{}, err
	}
	if len(words) == 0 || words[0] != "curl" && words[0] != "curl.exe" {
		return Request{}, errors.New("enter a command starting with curl")
	}

	request := Request{Headers: make(map[string]string)}
	seenHeaders := make(map[string]bool)
	var data, jsonData []string
	var explicitMethod string
	var head, get, compressed, formEncoded bool
	words = words[1:]
	for len(words) != 0 {
		option := words[0]
		words = words[1:]
		switch option {
		case "--disable", "-q", "--globoff", "-g", "--path-as-is", "--silent", "-s", "--show-error", "-S":
			continue
		case "--head", "-I":
			head = true
			continue
		case "--get", "-G":
			get = true
			continue
		case "--location", "-L":
			request.FollowRedirects = true
			continue
		case "--compressed":
			compressed = true
			continue
		case "--no-compressed":
			compressed = false
			continue
		}
		if !strings.HasPrefix(option, "-") {
			if request.URL != "" {
				return Request{}, errors.New("cURL import supports one URL")
			}
			request.URL = option
			continue
		}
		var value string
		var inline bool
		if strings.HasPrefix(option, "--") {
			option, value, inline = strings.Cut(option, "=")
		}
		switch option {
		case "--url", "--request", "-X", "--header", "-H", "--data-raw", "--data", "-d", "--data-ascii", "--data-binary", "--data-urlencode", "--json", "--max-time", "--proto":
		default:
			return Request{}, errors.New("unsupported cURL option")
		}
		if !inline {
			if len(words) == 0 {
				return Request{}, errors.New("missing cURL option value")
			}
			value, words = words[0], words[1:]
		}
		switch option {
		case "--url":
			if request.URL != "" {
				return Request{}, errors.New("cURL import supports one URL")
			}
			request.URL = value
		case "--request", "-X":
			if value == "" {
				return Request{}, errors.New("cURL method cannot be empty")
			}
			explicitMethod = value
		case "--header", "-H":
			name, content, found := strings.Cut(value, ":")
			if !found {
				name, found = strings.CutSuffix(value, ";")
			}
			if !found || name == "" || strings.ContainsAny(name, " \t\r\n") || strings.ContainsAny(content, "\r\n") {
				return Request{}, errors.New("cURL headers must use Name: value or Name;")
			}
			name = http.CanonicalHeaderKey(name)
			if seenHeaders[name] {
				return Request{}, errors.New("cURL import does not support repeated header names")
			}
			seenHeaders[name] = true
			content = strings.TrimSpace(content)
			if !strings.Contains(value, ":") || content != "" {
				request.Headers[name] = content
			}
		case "--data-raw", "--data", "-d", "--data-ascii", "--data-binary":
			if option != "--data-raw" && strings.HasPrefix(value, "@") {
				return Request{}, errors.New("cURL file references cannot be imported")
			}
			data = append(data, value)
		case "--json":
			if strings.HasPrefix(value, "@") {
				return Request{}, errors.New("cURL file references cannot be imported")
			}
			jsonData = append(jsonData, value)
		case "--data-urlencode":
			name, content, named := strings.Cut(value, "=")
			if !named {
				if strings.ContainsRune(value, '@') {
					return Request{}, errors.New("cURL file references cannot be imported")
				}
				name, content = "", value
			}
			if strings.ContainsRune(name, '@') {
				return Request{}, errors.New("cURL file references cannot be imported")
			}
			encoded := url.QueryEscape(content)
			if name != "" {
				encoded = name + "=" + encoded
			}
			formEncoded = true
			data = append(data, encoded)
		case "--max-time":
			seconds, err := strconv.ParseFloat(value, 64)
			if err != nil || !(seconds >= 0.001 && seconds <= 300) {
				return Request{}, errors.New("cURL timeout must be between 1 ms and 300 seconds")
			}
			request.HTTPTimeoutMS = int(seconds*1000 + 0.5)
		case "--proto":
			if value != "=http,https" {
				return Request{}, errors.New("unsupported cURL protocol restriction")
			}
		}
	}
	if compressed {
		// Let the execution transport negotiate an encoding it can decode.
		delete(request.Headers, "Accept-Encoding")
	}
	if len(jsonData) != 0 {
		if len(data) != 0 || get || head {
			return Request{}, errors.New("cURL import supports --json on its own as request data")
		}
		data = append(data, strings.Join(jsonData, ""))
		for _, name := range []string{"Content-Type", "Accept"} {
			if !seenHeaders[name] {
				request.Headers[name] = "application/json"
				seenHeaders[name] = true
			}
		}
	}
	request.Method = explicitMethod
	if head {
		if len(data) != 0 && !get || explicitMethod != "" && explicitMethod != http.MethodHead {
			return Request{}, errors.New("cURL HEAD cannot be combined with request data or another method")
		}
		request.Method = http.MethodHead
	}
	if len(data) != 0 && !get {
		request.Body = strings.Join(data, "&")
		if request.Method == "" {
			request.Method = http.MethodPost
		}
		if !seenHeaders["Content-Type"] {
			request.Headers["Content-Type"] = "application/x-www-form-urlencoded"
		}
	}
	parsed, err := http.NewRequest(request.Method, request.URL, nil)
	if err != nil || parsed.URL.Hostname() == "" || parsed.URL.User != nil ||
		(parsed.URL.Scheme != "http" && parsed.URL.Scheme != "https") {
		return Request{}, errors.New("cURL import requires a valid HTTP or HTTPS URL and method")
	}
	request.Method = parsed.Method
	if get && len(data) != 0 {
		if parsed.URL.RawQuery != "" {
			parsed.URL.RawQuery += "&"
		}
		parsed.URL.RawQuery += strings.Join(data, "&")
		request.URL = parsed.URL.String()
	}
	if formEncoded || get {
		query := request.Body
		if get {
			query = parsed.URL.RawQuery
		}
		values, err := url.ParseQuery(query)
		if err != nil {
			return Request{}, errors.New("cURL form data or query has invalid escaping")
		}
		for key := range values {
			if sensitiveKey.MatchString(key) {
				return Request{}, errors.New("remove possible credentials before importing a cURL command")
			}
		}
	}
	request.Name = parsed.URL.EscapedPath()
	if request.Name == "" {
		request.Name = "/"
	}
	if request.containsSensitiveData() || hasPrivateKey(request.Body) {
		return Request{}, errors.New("remove possible credentials before importing a cURL command")
	}
	return request, nil
}

func curlWords(source string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote byte
	started := false
	for index := 0; index < len(source); index++ {
		char := source[index]
		switch {
		case quote == '\'':
			if char == quote {
				quote = 0
			} else {
				word.WriteByte(char)
			}
		case char == '\\':
			if index+1 == len(source) {
				return nil, errors.New("incomplete cURL escape")
			}
			next := source[index+1]
			if next == '\r' && index+2 < len(source) && source[index+2] == '\n' {
				index += 2
				continue
			}
			if next == '\n' {
				index++
				continue
			}
			if quote == 0 || strings.ContainsRune("\"\\$`", rune(next)) {
				char = next
				index++
			}
			word.WriteByte(char)
			started = true
		case quote != 0:
			switch char {
			case '"':
				quote = 0
			case '$', '`':
				return nil, errors.New("shell expansions are not supported in cURL imports")
			default:
				word.WriteByte(char)
			}
		case char == '\'' || char == '"':
			quote, started = char, true
		case char == ' ' || char == '\t':
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
		case strings.ContainsRune("\r\n;&|<>()$`#", rune(char)):
			return nil, errors.New("cURL import accepts one command with literal arguments")
		default:
			word.WriteByte(char)
			started = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unclosed quote in cURL command")
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}
