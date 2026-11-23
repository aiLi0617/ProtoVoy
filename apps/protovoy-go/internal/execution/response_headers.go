package execution

import (
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

const ResponseHeadersMetadata = "X-ProtoVoy-Response-Headers"

var sensitiveResponseHeader = regexp.MustCompile(`(?i)(authorization|cookie|token|secret|api[-_]?key|password|passwd)`)

func IsSensitiveHTTPHeader(name string) bool {
	return sensitiveResponseHeader.MatchString(name)
}

// PreviewResponseHeaders returns a bounded, ordered view of upstream metadata.
func PreviewResponseHeaders(headers http.Header) [][2]string {
	const limit = 2048
	remaining := limit
	var preview [][2]string
	for _, name := range slices.Sorted(maps.Keys(headers)) {
		for _, value := range headers[name] {
			if IsSensitiveHTTPHeader(name) {
				value = "[redacted]"
			}
			size := len(name) + len(value) + 4
			if size > remaining {
				continue
			}
			remaining -= size
			preview = append(preview, [2]string{name, strings.ToValidUTF8(value, "�")})
		}
	}
	return preview
}
