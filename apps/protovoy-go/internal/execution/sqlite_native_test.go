//go:build !js

package execution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLiteFileAndReadOnlyMode(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sqlite fixture.db")
	target := "sqlite:///" + strings.TrimPrefix(filepath.ToSlash(file), "/")
	run := func(target, statement string, args ...json.RawMessage) (string, error) {
		t.Helper()
		endpoint, err := NewSQLEndpoint(target, "", "")
		if err != nil {
			return "", err
		}
		request, err := endpoint.Prepare(SQLRequest{Statement: statement, Parameters: args, Transactional: true})
		if err != nil {
			return "", err
		}
		var output string
		err = request.Execute(context.Background(), func() error { return nil }, func(value any) error {
			output = string(value.(json.RawMessage))
			return nil
		})
		return output, err
	}
	if _, err := run(target, "CREATE TABLE items (value INTEGER)"); err == nil {
		t.Fatal("default mode created a missing SQLite file")
	}
	if _, err := run(target+"?mode=rwc", "CREATE TABLE items (value INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(target, "INSERT INTO items VALUES (?)", json.RawMessage("9007199254740993")); err != nil {
		t.Fatal(err)
	}
	if data, err := run(target+"?mode=ro", "SELECT value, X'00FF' AS blob FROM items"); err != nil ||
		!strings.Contains(data, `"value":9007199254740993`) || !strings.Contains(data, `"blob":{"base64":"AP8="}`) {
		t.Fatalf("unexpected SQLite result: %s (%v)", data, err)
	}
	if _, err := run(target+"?mode=ro", "DELETE FROM items"); err == nil {
		t.Fatal("SQLite read-only mode accepted a write")
	}
	other := filepath.Join(t.TempDir(), "other.db")
	for _, statement := range []string{
		"ATTACH DATABASE ? AS external",
		"VACUUM INTO ?",
	} {
		data, _ := json.Marshal(other)
		if _, err := run(target, statement, data); err == nil {
			t.Fatalf("SQLite accepted %s", statement)
		}
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("an unauthorized SQLite file was created: %v", err)
	}
	if _, err := run(target, "PRAGMA writable_schema = ON"); err == nil {
		t.Fatal("SQLite accepted a restricted PRAGMA")
	}
	if data, err := run(target, "PRAGMA table_info(items)"); err != nil || !strings.Contains(data, `"name":"value"`) {
		t.Fatalf("SQLite introspection failed: %s (%v)", data, err)
	}
}

func TestSQLiteMemoryTargetAndURIRestrictions(t *testing.T) {
	endpoint, err := NewSQLEndpoint(SQLiteMemoryTarget, "", "")
	if err != nil {
		t.Fatal(err)
	}
	request, err := endpoint.Prepare(SQLRequest{Statement: "SELECT 42 AS answer"})
	if err != nil {
		t.Fatal(err)
	}
	if err := request.Execute(context.Background(), func() error { return nil }, func(value any) error {
		if data := string(value.(json.RawMessage)); !strings.Contains(data, `"answer":42`) {
			t.Fatalf("unexpected in-memory response: %s", data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		"sqlite::memory:?cache=shared", "sqlite:test.db?vfs=custom", "sqlite:test.db?_pragma=trusted_schema(1)",
		"sqlite:test.db?mode=ro&mode=rwc", "sqlite:test.db?mode=memory",
	} {
		if _, err := NewSQLEndpoint(target, "", ""); err == nil {
			t.Errorf("accepted unsupported SQLite target %q", target)
		}
	}
}
