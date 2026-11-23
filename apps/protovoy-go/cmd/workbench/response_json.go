package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

func formatResponseJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return ""
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ""
		}
		switch token {
		case json.Delim('{'), json.Delim('['):
			depth++
			if depth > 64 {
				return ""
			}
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
	}
	var formatted bytes.Buffer
	if json.Indent(&formatted, []byte(raw), "", "  ") != nil || formatted.String() == raw {
		return ""
	}
	return formatted.String()
}
