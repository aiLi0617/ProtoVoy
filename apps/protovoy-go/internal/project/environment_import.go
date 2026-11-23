package project

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

const MaxEnvironmentImportBytes = 1 << 20

type EnvironmentImport struct {
	Environments []Environment
	SecretRefs   int
	Defaults     int
}

// ReadLegacyEnvironments reads a single record or the list response from the
// legacy Local Agent. Keychain references and unsupported HTTP defaults are
// counted separately for the migration report.
func ReadLegacyEnvironments(data []byte) (EnvironmentImport, error) {
	if len(data) > MaxEnvironmentImportBytes {
		return EnvironmentImport{}, errors.New("environment file exceeds 1 MiB")
	}
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if len(data) == 0 {
		return EnvironmentImport{}, errors.New("empty environment file")
	}
	if data[0] == '{' {
		data = append(append([]byte{'['}, data...), ']')
	}
	var records []struct {
		ID, ProjectID, Name, UpdatedAt string
		Variables                      map[string]*string
		GlobalParameters               []execution.HTTPDefault
		SecretRefs                     []string
	}
	if !decodeLegacy(data, &records) || len(records) == 0 || len(records) > MaxEnvironments {
		return EnvironmentImport{}, errors.New("expected 1 to 64 legacy environment records")
	}
	var result EnvironmentImport
	for i, record := range records {
		name := strings.TrimSpace(record.Name)
		if name == "" || len(name) > 128 || strings.ContainsAny(name, "\r\n") || record.Variables == nil {
			return EnvironmentImport{}, fmt.Errorf("environment %d needs a valid name and variables object", i+1)
		}
		for _, env := range result.Environments {
			if strings.EqualFold(env.Name, name) {
				return EnvironmentImport{}, errors.New("the file contains duplicate environment names")
			}
		}
		variables := make(map[string]string, len(record.Variables))
		for key, value := range record.Variables {
			if value == nil {
				return EnvironmentImport{}, fmt.Errorf("environment %d has a null variable value", i+1)
			}
			variables[key] = *value
		}
		if sensitiveVariables(variables) {
			return EnvironmentImport{}, fmt.Errorf("environment %d contains possible credentials", i+1)
		}
		for _, value := range variables {
			if hasPrivateKey(value) || sensitiveBody.MatchString(value) {
				return EnvironmentImport{}, fmt.Errorf("environment %d contains possible credentials", i+1)
			}
		}
		values, err := ParseVariables(FormatVariables(variables))
		if err != nil || !maps.Equal(values, variables) {
			return EnvironmentImport{}, fmt.Errorf("environment %d contains variables that the editor cannot represent", i+1)
		}
		env := Environment{Name: name, Variables: values}
		for _, parameter := range record.GlobalParameters {
			switch parameter.Location {
			case "header", "query", "body":
				if strings.TrimSpace(parameter.Name) == "" || unsafeHTTPDefault(parameter) {
					return EnvironmentImport{}, fmt.Errorf("environment %d has an invalid or sensitive HTTP default", i+1)
				}
				env.HTTPDefaults = append(env.HTTPDefaults, parameter)
			default:
				result.Defaults++
			}
		}
		result.Environments = append(result.Environments, env)
		result.SecretRefs += len(record.SecretRefs)
	}
	return result, nil
}

func unsafeHTTPDefault(parameter execution.HTTPDefault) bool {
	name := strings.TrimSpace(parameter.Name)
	return sensitiveKey.MatchString(name) || sensitiveBody.MatchString(parameter.Value) || hasPrivateKey(parameter.Value)
}
