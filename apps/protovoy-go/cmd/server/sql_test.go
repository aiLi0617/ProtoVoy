package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
	"protovoy.dev/protovoy/internal/execution"
)

func TestSQLBridgeTimesOutRunningSQLiteStatement(t *testing.T) {
	const addr = "127.0.0.1:39300"
	target := execution.SQLiteMemoryTarget
	database, err := execution.NewSQLEndpoint(target, "", "")
	if err != nil {
		t.Fatal(err)
	}
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{SQL: &sqlEndpoint{target: target, database: database}})
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://"+addr+execution.SQLBridgePath, strings.NewReader(body))
		r.Header.Set("Origin", "http://"+addr)
		r.Header.Set("X-ProtoVoy-Preview", "1")
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		return w
	}
	body := `{"timeoutMs":75,"sql":"WITH RECURSIVE seq(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM seq WHERE x<100000000) SELECT sum(x) FROM seq"}`
	if w := request(body); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "SQL execution timed out") ||
		strings.Contains(w.Body.String(), `"type":"completed"`) {
		t.Fatalf("unexpected timeout response: %d %s", w.Code, w.Body.String())
	}
	if w := request(`{"timeoutMs":300001,"sql":"SELECT 1"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("out-of-range timeout accepted: %d", w.Code)
	}
}

func TestHostedSQLiteCannotUseTeamDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "team.db")
	target := "sqlite:" + filepath.ToSlash(path) + "?mode=rwc"
	if err := checkTeamSQLIsolation(path, target); err == nil {
		t.Fatal("accepted the same database path before creation")
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		sidecar := path + suffix
		if err := checkTeamSQLIsolation(path, "sqlite:"+filepath.ToSlash(sidecar)); err == nil {
			t.Errorf("accepted team sidecar %s as SQL target", suffix)
		}
		if err := checkTeamSQLIsolation(sidecar, target); err == nil {
			t.Errorf("accepted SQL sidecar %s as team database", suffix)
		}
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(path), "alias.db")
	if err := os.Link(path, alias); err == nil {
		if err := checkTeamSQLIsolation(path, "sqlite:"+filepath.ToSlash(alias)); err == nil {
			t.Fatal("accepted a hard link to the team database")
		}
	}
	if err := checkTeamSQLIsolation(path, "sqlite:"+filepath.ToSlash(filepath.Join(filepath.Dir(path), "jobs.db"))); err != nil {
		t.Fatal(err)
	}
}

func TestSQLBridgeExecutesAgainstConfiguredSQLiteFile(t *testing.T) {
	const addr = "127.0.0.1:39300"
	target := "sqlite:" + filepath.ToSlash(filepath.Join(t.TempDir(), "jobs.db")) + "?mode=rwc"
	database, err := execution.NewSQLEndpoint(target, "", "")
	if err != nil {
		t.Fatal(err)
	}
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{SQL: &sqlEndpoint{target: target, database: database}})
	for _, tc := range []struct{ body, result string }{
		{`{"sql":"CREATE TABLE jobs (id INTEGER PRIMARY KEY)"}`, `"rowsAffected":0`},
		{`{"sql":"INSERT INTO jobs (id) VALUES (?)","parameters":[42],"transactional":true}`, `"lastInsertId":42`},
		{`{"sql":"SELECT id FROM jobs"}`, `"rows":[{"id":42}]`},
	} {
		r := httptest.NewRequest(http.MethodPost, "http://"+addr+execution.SQLBridgePath, strings.NewReader(tc.body))
		r.Header.Set("Origin", "http://"+addr)
		r.Header.Set("X-ProtoVoy-Preview", "1")
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), tc.result) ||
			!strings.Contains(w.Body.String(), `{"type":"completed"}`) {
			t.Fatalf("unexpected SQLite result: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestSQLBridgeRequiresAuthorizedSameOriginAndFixedTarget(t *testing.T) {
	const origin = "https://workbench.example.com"
	access, err := configureWebAccess("127.0.0.1:39300", origin, "test-cert", "test-key", "operator", fixtureWebPassword)
	if err != nil {
		t.Fatal(err)
	}
	configured := handler(access, t.TempDir(), "", "", socketTargets{SQL: &sqlEndpoint{target: "postgres://db.example.com/protovoy"}})
	disabled := handler(access, t.TempDir(), "", "")
	for _, tc := range []struct {
		name, requestOrigin, marker, body string
		auth, disabled                    bool
		want                              int
	}{
		{"anonymous", origin, "1", "{}", false, false, http.StatusUnauthorized},
		{"cross-origin", "https://other.example.com", "1", "{}", true, false, http.StatusForbidden},
		{"no marker", origin, "", "{}", true, false, http.StatusForbidden},
		{"disabled", origin, "1", "{}", true, true, http.StatusServiceUnavailable},
		{"different database", origin, "1", `{"expectedTarget":"postgres://db.example.com/other"}`, true, false, http.StatusConflict},
		{"different host", origin, "1", `{"expectedTarget":"postgres://other.example.com/protovoy"}`, true, false, http.StatusConflict},
		{"credentials", origin, "1", `{"sql":"SELECT 1","password":"other"}`, true, false, http.StatusBadRequest},
		{"mismatched credential reference", origin, "1", `{"sql":"SELECT 1","expectedPasswordRef":"other"}`, true, false, http.StatusBadRequest},
		{"TLS override", origin, "1", `{"sql":"SELECT 1","sslmode":"disable"}`, true, false, http.StatusBadRequest},
		{"empty statement", origin, "1", `{"sql":" "}`, true, false, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, origin+execution.SQLBridgePath, strings.NewReader(tc.body))
			r.Header.Set("Origin", tc.requestOrigin)
			r.Header.Set("X-ProtoVoy-Preview", tc.marker)
			if tc.auth {
				r.SetBasicAuth("operator", fixtureWebPassword)
			}
			w := httptest.NewRecorder()
			api := configured
			if tc.disabled {
				api = disabled
			}
			api.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestSQLBridgeStreamsBoundPostgreSQLResult(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_ = listener.SetDeadline(time.Now().Add(5 * time.Second))
	peerDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			err = sqlPostgreSQLPeer(conn)
		}
		peerDone <- err
	}()

	target := "postgres://" + listener.Addr().String() + "/jobs?sslmode=disable"
	database, err := execution.NewSQLEndpoint(target, "fixture-user", "")
	if err != nil {
		t.Fatal(err)
	}
	const addr = "127.0.0.1:39300"
	api := handler(loopbackAccess(addr), t.TempDir(), "", "", socketTargets{
		SQL: &sqlEndpoint{target: target, database: database, passwordRef: "jobs"},
	})
	r := httptest.NewRequest(http.MethodPost, "http://"+addr+execution.SQLBridgePath,
		strings.NewReader(`{"sql":"SELECT $1::int8 AS value","parameters":[42],"expectedPasswordRef":"jobs"}`))
	r.Header.Set("Origin", "http://"+addr)
	r.Header.Set("X-ProtoVoy-Preview", "1")
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if err := <-peerDone; err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/x-ndjson" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected SQL response status or headers: %d %v", w.Code, w.Header())
	}
	decoder := json.NewDecoder(w.Body)
	for _, eventType := range []string{"connected", "message", "completed"} {
		var event execution.BrokerBridgeEvent
		if err := decoder.Decode(&event); err != nil || event.Type != eventType {
			t.Fatalf("expected %s, got %+v (%v)", eventType, event, err)
		}
		if eventType == "message" && string(event.Message) != `{"columns":["value"],"rows":[{"value":42}],"truncated":false}` {
			t.Fatalf("unexpected query result %s", event.Message)
		}
	}
}

func sqlPostgreSQLPeer(conn net.Conn) error {
	peer := pgproto3.NewBackend(conn, conn)
	message, err := peer.ReceiveStartupMessage()
	startup, ok := message.(*pgproto3.StartupMessage)
	if err != nil || !ok || startup.Parameters["database"] != "jobs" || startup.Parameters["user"] != "fixture-user" {
		return fmt.Errorf("unexpected PostgreSQL startup: %T (%v)", message, err)
	}
	peer.Send(&pgproto3.AuthenticationOk{})
	peer.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := peer.Flush(); err != nil {
		return err
	}
	var result []byte
	for {
		message, err := peer.Receive()
		if err != nil {
			return err
		}
		switch message := message.(type) {
		case *pgproto3.Parse:
			if message.Query != "SELECT $1::int8 AS value" {
				return fmt.Errorf("unexpected SQL statement: %q", message.Query)
			}
			peer.Send(&pgproto3.ParseComplete{})
		case *pgproto3.Describe:
			peer.Send(&pgproto3.ParameterDescription{ParameterOIDs: []uint32{20}})
			peer.Send(&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{{Name: []byte("value"), DataTypeOID: 20, DataTypeSize: 8}}})
		case *pgproto3.Bind:
			if len(message.Parameters) != 1 || len(message.Parameters[0]) != 8 || binary.BigEndian.Uint64(message.Parameters[0]) != 42 {
				return fmt.Errorf("expected bound int8 value")
			}
			result = []byte("42")
			if len(message.ResultFormatCodes) == 1 && message.ResultFormatCodes[0] == 1 {
				result = binary.BigEndian.AppendUint64(nil, 42)
			}
			peer.Send(&pgproto3.BindComplete{})
		case *pgproto3.Execute:
			peer.Send(&pgproto3.DataRow{Values: [][]byte{result}})
			peer.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})
		case *pgproto3.Sync:
			peer.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
			if err := peer.Flush(); err != nil {
				return err
			}
		case *pgproto3.Terminate:
			return nil
		default:
			return fmt.Errorf("unexpected PostgreSQL message: %T", message)
		}
	}
}
