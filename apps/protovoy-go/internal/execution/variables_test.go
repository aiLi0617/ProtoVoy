package execution

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResolveVariablesPrecedenceAndDynamicValues(t *testing.T) {
	request := HTTPPayload{
		Headers: [][2]string{{"X-Test", "{{ value }}"}},
		Body:    "{{$uuid}} {{$uuid}} {{$timestamp}} {{$isoTimestamp}} {{blank}}",
	}
	target, resolved, err := request.ResolveVariables("{{url}}", map[string]string{"url": "global", "value": "global"},
		map[string]string{"url": "environment", "value": "environment"},
		map[string]string{"url": "collection", "value": "collection"},
		map[string]string{"value": "request", "blank": "", "$uuid": "reserved"})
	if err != nil || target != "collection" || resolved.Headers[0][1] != "request" || request.Headers[0][1] != "{{ value }}" {
		t.Fatalf("unexpected variable resolution: %v, %q, %v", err, target, resolved.Headers)
	}
	parts := strings.Split(resolved.Body, " ")
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if len(parts) != 5 || !uuid.MatchString(parts[0]) || !uuid.MatchString(parts[1]) || parts[0] == parts[1] || parts[4] != "" {
		t.Fatalf("unexpected dynamic values: %q", resolved.Body)
	}
	iso, isoErr := time.Parse(time.RFC3339Nano, parts[3])
	unix, unixErr := strconv.ParseInt(parts[2], 10, 64)
	if isoErr != nil || unixErr != nil || iso.Unix() != unix {
		t.Fatalf("timestamps differ: %q", resolved.Body)
	}
}

func TestResolveVariablesRejectsMissingOrOversizeValues(t *testing.T) {
	payload := HTTPPayload{Body: "{{name}}{{name}}"}
	for _, values := range []map[string]string{
		nil,
		{"name": strings.Repeat("a", 5<<20)},
	} {
		if _, _, err := payload.ResolveVariables("https://example.test", values); err == nil {
			t.Fatal("request should fail before execution")
		}
	}
}
