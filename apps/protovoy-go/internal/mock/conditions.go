package mock

import (
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

func (conditions Conditions) validate() error {
	for _, condition := range conditions.Conditions {
		switch condition.Source {
		case "query", "path", "header", "cookie", "body", "ip":
		default:
			return errors.New("mock condition has an unknown source")
		}
		switch condition.Operator {
		case "exists", "notExists":
			continue
		case "equals", "notEquals", "contains", "notContains",
			"greaterThan", "greaterOrEqual", "lessThan", "lessOrEqual", "matches":
		default:
			return errors.New("mock condition has an unknown operator")
		}
		if condition.Value == nil {
			return errors.New("mock comparison condition requires a value")
		}
		if condition.Operator == "matches" {
			if _, err := regexp.Compile(*condition.Value); err != nil {
				return errors.New("mock condition has an invalid regular expression")
			}
		}
		switch condition.Operator {
		case "greaterThan", "greaterOrEqual", "lessThan", "lessOrEqual":
			value, err := strconv.ParseFloat(*condition.Value, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return errors.New("mock numeric comparison requires a finite number")
			}
		}
	}
	return nil
}

func (conditions Conditions) matches(request Request, parameters map[string]string) bool {
	if !strings.Contains(request.Body, conditions.BodyContains) ||
		len(conditions.IPs) > 0 && !slices.Contains(conditions.IPs, request.ClientIP) {
		return false
	}
	for name, expected := range conditions.Query {
		if !compare(first(request.Query[name]), "equals", &expected) {
			return false
		}
	}
	for name, expected := range conditions.Headers {
		if !compare(first(request.Headers.Values(name)), "equals", &expected) {
			return false
		}
	}
	for name, expected := range conditions.Cookies {
		if !compare(lookup(request.Cookies, name), "equals", &expected) {
			return false
		}
	}

	var body any
	var bodyParsed, bodyValid bool
	for _, condition := range conditions.Conditions {
		var actual *string
		switch condition.Source {
		case "query":
			actual = first(request.Query[condition.Name])
		case "header":
			actual = first(request.Headers.Values(condition.Name))
		case "cookie":
			actual = lookup(request.Cookies, condition.Name)
		case "path":
			actual = lookup(parameters, condition.Name)
		case "ip":
			actual = &request.ClientIP
		case "body":
			if !bodyParsed {
				decoder := json.NewDecoder(strings.NewReader(request.Body))
				decoder.UseNumber()
				bodyValid = json.Valid([]byte(request.Body)) && decoder.Decode(&body) == nil
				bodyParsed = true
			}
			actual = bodyValue(condition.Name, request.Body, body, bodyValid)
		default:
			// Unknown sources have no value.
		}
		if !compare(actual, condition.Operator, condition.Value) {
			return false
		}
	}
	return true
}

func first(values []string) *string {
	if len(values) == 0 {
		return nil
	}
	return &values[0]
}

func lookup(values map[string]string, name string) *string {
	value, ok := values[name]
	if !ok {
		return nil
	}
	return &value
}

func compare(actual *string, operator string, expected *string) bool {
	switch operator {
	case "exists":
		return actual != nil
	case "notExists":
		return actual == nil
	}
	if actual == nil || expected == nil {
		return false
	}
	switch operator {
	case "equals":
		return *actual == *expected
	case "notEquals":
		return *actual != *expected
	case "contains":
		return strings.Contains(*actual, *expected)
	case "notContains":
		return !strings.Contains(*actual, *expected)
	case "matches":
		ok, err := regexp.MatchString(*expected, *actual)
		return err == nil && ok
	case "greaterThan", "greaterOrEqual", "lessThan", "lessOrEqual":
		return compareNumbers(*actual, operator, *expected)
	default:
		return false
	}
}

func compareNumbers(actual, operator, expected string) bool {
	left, leftErr := strconv.ParseFloat(actual, 64)
	right, rightErr := strconv.ParseFloat(expected, 64)
	if leftErr != nil || rightErr != nil {
		return false
	}
	switch operator {
	case "greaterThan":
		return left > right
	case "greaterOrEqual":
		return left >= right
	case "lessThan":
		return left < right
	default:
		return left <= right
	}
}

func bodyValue(path, raw string, value any, valid bool) *string {
	path = strings.TrimSpace(path)
	if !valid {
		if path == "" || path == "$" {
			return &raw
		}
		return nil
	}
	path = strings.TrimLeft(strings.TrimPrefix(path, "$"), ".")
	if path != "" {
		var ok bool
		value, ok = jsonPathValue(value, path)
		if !ok {
			return nil
		}
	}
	if text, ok := value.(string); ok {
		return &text
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	text := string(data)
	return &text
}

func jsonPathValue(value any, path string) (any, bool) {
	for segment := range strings.SplitSeq(path, ".") {
		field, indexes, indexed := strings.Cut(segment, "[")
		if field != "" || !indexed {
			object, ok := value.(map[string]any)
			if !ok {
				return nil, false
			}
			value, ok = object[field]
			if !ok {
				return nil, false
			}
		}
		for indexed {
			index, rest, closed := strings.Cut(indexes, "]")
			array, ok := value.([]any)
			offset, err := strconv.Atoi(index)
			if !closed || !ok || err != nil || offset < 0 || offset >= len(array) {
				return nil, false
			}
			value = array[offset]
			if rest == "" {
				break
			}
			indexes, indexed = strings.CutPrefix(rest, "[")
			if !indexed {
				return nil, false
			}
		}
	}
	return value, true
}
