package project

import (
	"fmt"
	"sort"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

const MaxEnvironments = 64

type Environment struct {
	Name         string                  `json:"name"`
	Variables    map[string]string       `json:"variables,omitempty"`
	HTTPDefaults []execution.HTTPDefault `json:"httpDefaults,omitempty"`
}

type VariableScopes struct {
	Global       map[string]string `json:"global,omitempty"`
	Collection   map[string]string `json:"collection,omitempty"`
	Environments []Environment     `json:"environments,omitempty"`
	Active       string            `json:"active,omitempty"`
}

func (scopes VariableScopes) ActiveVariables() map[string]string {
	return scopes.ActiveEnvironment().Variables
}

func (scopes VariableScopes) ActiveEnvironment() Environment {
	if scopes.Active != "" {
		for _, env := range scopes.Environments {
			if env.Name == scopes.Active {
				return env
			}
		}
	}
	return Environment{}
}

func (scopes VariableScopes) containsSensitiveData() bool {
	if sensitiveVariables(scopes.Global) || sensitiveVariables(scopes.Collection) {
		return true
	}
	for _, env := range scopes.Environments {
		if sensitiveVariables(env.Variables) {
			return true
		}
		for _, parameter := range env.HTTPDefaults {
			if unsafeHTTPDefault(parameter) {
				return true
			}
		}
	}
	return false
}

func sensitiveVariables(values map[string]string) bool {
	for key := range values {
		if sensitiveKey.MatchString(key) {
			return true
		}
	}
	return false
}

func FormatVariables(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var lines []string
	for _, key := range keys {
		lines = append(lines, key+"="+values[key])
	}
	return strings.Join(lines, "\n")
}

func ParseVariables(text string) (map[string]string, error) {
	values := make(map[string]string)
	for index, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, "{}") {
			return nil, fmt.Errorf("variable line %d needs a name and an equals sign", index+1)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("variable line %d repeats a name", index+1)
		}
		values[key] = value
	}
	return values, nil
}
