package execution

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var variablePattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

func ResolveTextVariables(input string, scopes ...map[string]string) (string, error) {
	resolver := newVariableResolver(scopes)
	value := resolver.text(input)
	return value, resolver.err
}

// ResolveVariables substitutes values at send time. Later scopes override
// earlier ones; reserved dynamic variables take precedence over all scopes.
func (payload HTTPPayload) ResolveVariables(target string, scopes ...map[string]string) (string, HTTPPayload, error) {
	return payload.ResolveHTTPDefaults(target, nil, scopes...)
}

func (payload HTTPPayload) ResolveHTTPDefaults(target string, defaults []HTTPDefault, scopes ...map[string]string) (string, HTTPPayload, error) {
	resolver := newVariableResolver(scopes)
	target = resolver.text(target)
	if len(payload.Multipart) == 0 {
		payload.Body = resolver.text(payload.Body)
	}
	payload.Headers = resolver.headers(payload.Headers)
	payload.Multipart = slices.Clone(payload.Multipart)
	for i := range payload.Multipart {
		part := &payload.Multipart[i]
		part.Name, part.Value = resolver.text(part.Name), resolver.text(part.Value)
		if part.FileName != nil {
			value := resolver.text(*part.FileName)
			part.FileName = &value
		}
		if part.ContentType != nil {
			value := resolver.text(*part.ContentType)
			part.ContentType = &value
		}
	}
	for _, parameter := range defaults {
		if parameter.Enabled != nil && !*parameter.Enabled {
			continue
		}
		name, value := resolver.text(strings.TrimSpace(parameter.Name)), resolver.text(parameter.Value)
		if resolver.err != nil {
			break
		}
		if name != "" {
			if parameter.Location == "query" && target == "" {
				return "", HTTPPayload{}, errors.New("an HTTP query default requires an explicit request URL")
			}
			target = payload.applyHTTPDefault(target, parameter.Location, name, value)
		}
	}
	if resolver.err != nil {
		return "", HTTPPayload{}, resolver.err
	}
	return target, payload, nil
}

type variableResolver struct {
	scopes    []map[string]string
	now       time.Time
	remaining int
	err       error
}

func newVariableResolver(scopes []map[string]string) *variableResolver {
	// Bound the aggregate output so a short project with repeated references
	// cannot expand a large variable into an arbitrarily large request.
	return &variableResolver{scopes: scopes, now: time.Now().UTC(), remaining: 8 << 20}
}

func (resolver *variableResolver) lookup(key string) (string, bool) {
	switch key {
	case "$uuid":
		var id [16]byte
		rand.Read(id[:])
		id[6] = id[6]&0x0f | 0x40
		id[8] = id[8]&0x3f | 0x80
		encoded := hex.EncodeToString(id[:])
		return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], true
	case "$timestamp":
		return strconv.FormatInt(resolver.now.Unix(), 10), true
	case "$isoTimestamp":
		return resolver.now.Format(time.RFC3339Nano), true
	}
	for i := len(resolver.scopes) - 1; i >= 0; i-- {
		if value, ok := resolver.scopes[i][key]; ok {
			return value, true
		}
	}
	return "", false
}

func (resolver *variableResolver) text(input string) string {
	var out strings.Builder
	appendText := func(value string) {
		if len(value) > resolver.remaining {
			resolver.err = errors.New("resolved request exceeds 8 MiB")
			return
		}
		out.WriteString(value)
		resolver.remaining -= len(value)
	}
	for resolver.err == nil {
		match := variablePattern.FindStringIndex(input)
		if match == nil {
			appendText(input)
			break
		}
		key := strings.TrimSpace(input[match[0]+2 : match[1]-2])
		value, ok := resolver.lookup(key)
		if !ok {
			resolver.err = errors.New("request has an unresolved variable")
			break
		}
		appendText(input[:match[0]])
		if resolver.err == nil {
			appendText(value)
		}
		input = input[match[1]:]
	}
	return out.String()
}

func (resolver *variableResolver) headers(headers [][2]string) [][2]string {
	resolved := make([][2]string, len(headers))
	for index, header := range headers {
		resolved[index] = [2]string{resolver.text(header[0]), resolver.text(header[1])}
	}
	return resolved
}
