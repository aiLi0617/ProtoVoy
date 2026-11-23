// Package mock implements ProtoVoy's mock scenario selection.
package mock

import (
	"cmp"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type Rule struct {
	Name         string            `json:"name,omitempty"`
	Source       string            `json:"source,omitempty"`
	ID           string            `json:"id"`
	ProjectKey   string            `json:"projectKey"`
	ServiceKey   string            `json:"serviceKey"`
	OperationID  string            `json:"operationId,omitempty"`
	ResponseID   string            `json:"responseId,omitempty"`
	Enabled      *bool             `json:"enabled,omitempty"`
	Method       string            `json:"method"`
	Path         string            `json:"path"`
	Priority     int               `json:"priority,omitempty"`
	Match        Conditions        `json:"matchConditions,omitzero"`
	Status       int               `json:"status"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         string            `json:"body"`
	DelayMS      uint64            `json:"delayMs,omitempty"`
	ErrorEvery   uint64            `json:"errorEvery,omitempty"`
	WSMessages   []string          `json:"wsMessages,omitempty"`
	WSEcho       bool              `json:"wsEcho,omitempty"`
	WSIntervalMS uint64            `json:"wsIntervalMs,omitempty"`
}

type Conditions struct {
	Query        map[string]string `json:"query,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Cookies      map[string]string `json:"cookies,omitempty"`
	IPs          []string          `json:"ips,omitempty"`
	BodyContains string            `json:"bodyContains,omitempty"`
	Conditions   []Condition       `json:"conditions,omitempty"`
}

type Condition struct {
	Source   string  `json:"source,omitempty"`
	Name     string  `json:"name,omitempty"`
	Operator string  `json:"operator"`
	Value    *string `json:"value"`
}

type Request struct {
	ProjectKey  string
	ServiceKey  string
	Method      string
	Path        *string
	OperationID string
	ResponseID  string
	ScenarioID  string
	Query       url.Values
	Headers     http.Header
	Cookies     map[string]string
	Body        string
	ClientIP    string
}

// Select returns a rule when the request has one winner, or the conflicting
// scenario IDs when the request needs an explicit selector.
func Select(rules []Rule, request Request) (*Rule, []string) {
	var candidates []*Rule
	operations := make(map[string]struct{})
	for i := range rules {
		rule := &rules[i]
		if rule.matches(request) {
			candidates = append(candidates, rule)
			if rule.OperationID != "" {
				operations[rule.OperationID] = struct{}{}
			}
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if len(operations) <= 1 || request.ScenarioID != "" || request.OperationID != "" {
		highest := slices.MaxFunc(candidates, func(a, b *Rule) int { return cmp.Compare(a.Priority, b.Priority) }).Priority
		candidates = slices.DeleteFunc(candidates, func(rule *Rule) bool { return rule.Priority < highest })
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	ids := make([]string, 0, len(candidates))
	for _, rule := range candidates {
		ids = append(ids, rule.ID)
	}
	slices.Sort(ids)
	return nil, ids
}

func (rule Rule) matches(request Request) bool {
	if rule.Enabled != nil && !*rule.Enabled ||
		rule.ProjectKey != request.ProjectKey || rule.ServiceKey != request.ServiceKey ||
		rule.Method != "*" && !strings.EqualFold(rule.Method, request.Method) ||
		request.OperationID != "" && rule.OperationID != request.OperationID ||
		request.ResponseID != "" && rule.ResponseID != request.ResponseID ||
		request.ScenarioID != "" && rule.ID != request.ScenarioID {
		return false
	}
	var parameters map[string]string
	if request.Path != nil {
		parameters = pathParameters(rule.Path, *request.Path)
		if parameters == nil {
			return false
		}
	}
	return rule.Match.matches(request, parameters)
}

func pathParameters(template, actual string) map[string]string {
	expected := strings.Split(strings.Trim(strings.TrimSpace(template), "/"), "/")
	received := strings.Split(strings.Trim(strings.TrimSpace(actual), "/"), "/")
	if len(expected) != len(received) {
		return nil
	}
	parameters := make(map[string]string)
	for i, segment := range expected {
		switch {
		case strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}"):
			parameters[segment[1:len(segment)-1]] = received[i]
		case strings.HasPrefix(segment, ":"):
			parameters[segment[1:]] = received[i]
		case segment != received[i]:
			return nil
		}
	}
	return parameters
}
