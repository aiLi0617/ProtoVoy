package main

import (
	"path/filepath"
	"testing"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func TestScheduledSQLUsesConfiguredDatabase(t *testing.T) {
	target := "sqlite:" + filepath.ToSlash(filepath.Join(t.TempDir(), "queries.db")) + "?mode=rwc"
	database, err := execution.NewSQLEndpoint(target, "", "")
	if err != nil {
		t.Fatal(err)
	}
	execute := scheduledSQLExecutor(&sqlEndpoint{target: target, database: database, passwordRef: "database"})
	request := project.Request{URL: "{{database}}", Variables: map[string]string{"database": target, "value": "42"}, SQL: &project.SQLSettings{RowLimit: 100, TimeoutMS: 2000}}
	for _, statement := range []string{"CREATE TABLE jobs(id INTEGER PRIMARY KEY)", "INSERT INTO jobs VALUES ({{value}})", "SELECT id FROM jobs"} {
		request.Body = statement
		result, err := execute(t.Context(), request)
		if err != nil || !result.Success {
			t.Fatalf("%s: %+v %v", statement, result, err)
		}
	}
	request.Body = "INSERT INTO jobs VALUES (43)"
	for _, change := range []func(*project.Request){
		func(r *project.Request) { r.URL = "sqlite::memory:" },
		func(r *project.Request) { r.SQL.PasswordRef = "wrong" },
		func(r *project.Request) { r.SQL.RowLimit = 0 },
	} {
		input := request
		settings := *request.SQL
		input.SQL = &settings
		change(&input)
		if result, err := execute(t.Context(), input); err == nil || result.Success {
			t.Fatal("invalid SQL request accepted")
		}
	}
	// If a rejected request ran, the unique-key insertion would now fail.
	if result, err := execute(t.Context(), request); err != nil || !result.Success {
		t.Fatalf("rejected input modified database: %+v %v", result, err)
	}
	request.Body = "WITH RECURSIVE seq(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM seq WHERE x<100000000) SELECT sum(x) FROM seq"
	request.SQL.TimeoutMS = 25
	if result, err := execute(t.Context(), request); err == nil || result.Success {
		t.Fatal("SQL deadline did not interrupt execution")
	}
}
