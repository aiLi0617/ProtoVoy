package main

import (
	"bytes"
	"testing"

	"github.com/vivaui/viva/text"
	"protovoy.dev/protovoy/internal/project"
)

func TestSQLCollectionRoundTripWithoutPassword(t *testing.T) {
	w := newWorkbench()
	w.loadRequest(0, defaultSQLRequest())
	w.target.SetText("postgres://localhost/jobs")
	w.requestBody.input.Editor = text.NewEditor("SELECT $1")
	w.sql.parameters.input.Editor = text.NewEditor(`[9007199254740993]`)
	w.sql.limit.SetText("3")
	w.sql.timeout.SetText("1250")
	w.sql.transactional = true
	w.sql.username.SetText("reader")
	w.sql.password.SetText("sql-fixture-password")
	w.sql.passwordRef.SetText("local-db")
	w.saveDraft()
	if w.collection.selected != 1 {
		t.Fatalf("could not save SQL request: %s", w.collection.message)
	}
	portable, err := project.Encode(*w.collection.project)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := project.EncodeWorkspace(project.Workspace{Draft: w.draft(), Project: w.collection.project, Selected: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{portable, workspace} {
		if bytes.Contains(data, []byte("sql-fixture-password")) || !bytes.Contains(data, []byte("local-db")) {
			t.Fatal("SQL persistence lost the reference or included the password")
		}
	}
	restored, err := project.Read(bytes.NewReader(portable))
	if err != nil {
		t.Fatal(err)
	}
	other := newWorkbench()
	other.loadRequest(1, restored.Requests[0])
	if other.draft() != w.draft() || other.sql.password.Text() != "" {
		t.Fatal("SQL request did not round-trip")
	}
}

func TestImportLegacySQLTimeout(t *testing.T) {
	input := `{"protocolId":"sql","target":"sqlite::memory:","timeoutMs":1500,"payload":{"type":"raw","value":{"sql":"SELECT 1"}}}`
	imported, err := project.Read(bytes.NewBufferString(input))
	if err != nil {
		t.Fatal(err)
	}
	w := newWorkbench()
	w.loadRequest(0, imported.Requests[0])
	settings, err := sqlSettings(w.draft())
	if err != nil || settings.TimeoutMS != 1500 {
		t.Fatalf("legacy SQL timeout did not load: %d (%v)", settings.TimeoutMS, err)
	}
}
