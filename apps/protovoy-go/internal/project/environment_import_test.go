package project

import "testing"

func TestReadLegacyEnvironmentsPreservesVariablesAndReportsOmissions(t *testing.T) {
	const record = `{"id":"env-fixture","projectId":"default-project","name":"Staging","variables":{"region":"east","feature":"key=value"},"globalParameters":[{"location":"header","name":"X-Region","value":"{{region}}"},{"location":"cookie","name":"session","value":"fixture"}],"secretRefs":["legacy-key"],"updatedAt":"2026-01-01T00:00:00Z"}`
	for _, file := range []string{record, "[" + record + "]"} {
		imported, err := ReadLegacyEnvironments([]byte(file))
		if err != nil || len(imported.Environments) != 1 {
			t.Fatalf("could not import environment: %v", err)
		}
		env := imported.Environments[0]
		if env.Name != "Staging" || env.Variables["region"] != "east" || env.Variables["feature"] != "key=value" ||
			imported.SecretRefs != 1 || imported.Defaults != 1 || len(env.HTTPDefaults) != 1 || env.HTTPDefaults[0].Name != "X-Region" {
			t.Fatal("import lost variables or omission counts")
		}
	}
}

func TestReadLegacyEnvironmentsRejectsUnsafeOrLossyVariables(t *testing.T) {
	for _, variables := range []string{
		`{"api_key":"fixture"}`,
		`{"value":"-----BEGIN PRIVATE KEY-----"}`,
		`{"value":"line one\nline two"}`,
		`{"value":null}`,
	} {
		if _, err := ReadLegacyEnvironments([]byte(`{"name":"Staging","variables":` + variables + `}`)); err == nil {
			t.Fatal("import accepted unsafe or unrepresentable variables")
		}
	}
}
