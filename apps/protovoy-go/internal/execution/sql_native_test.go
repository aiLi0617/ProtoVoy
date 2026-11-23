//go:build !js

package execution

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestSQLRequestPreparesBoundValuesAndRestrictsConnectionOptions(t *testing.T) {
	request := SQLRequest{
		Statement: "SELECT $1, $2, $3, $4",
		Parameters: []json.RawMessage{
			json.RawMessage(`9007199254740993`), json.RawMessage(`"{{value}}"`),
			json.RawMessage(`{"nested":"{{value}}"}`), json.RawMessage(`null`),
		},
	}
	resolved, err := (SQLBridgeRequest{ExpectedTarget: "postgres://{{host}}/test?sslmode=disable", SQLRequest: request}).
		ResolveVariables(map[string]string{"host": "localhost", "value": "'quoted'"})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := NewSQLEndpoint(resolved.ExpectedTarget, "tester", "password")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := endpoint.Prepare(resolved.SQLRequest)
	want := []any{int64(9007199254740993), "'quoted'", `{"nested":"'quoted'"}`, nil}
	if err != nil || !reflect.DeepEqual(prepared.args, want) || prepared.rowLimit != 500 {
		t.Fatalf("unexpected SQL preparation: args=%v, error=%v", prepared.args, err)
	}
	if _, err := NewSQLEndpoint("mysql://localhost/test", "tester", "password"); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		"postgres://user:password@localhost/test",
		"postgres://localhost/test?sslrootcert=local.pem",
		"postgres://localhost/test?host=other",
		"postgres://localhost/test?sslmode=verify-full&sslmode=disable",
		"mysql://localhost/test?ssl-mode=DISABLED",
		"mysql://localhost/test?allowAllFiles=true",
	} {
		if _, err := sqlConnector(target, "", ""); err == nil {
			t.Error("accepted an unsafe SQL connection option")
		}
	}
}

func TestSQLTransactionAndBoundedResult(t *testing.T) {
	for _, test := range []struct {
		name string
		rows int64
		fail bool
		want string
	}{
		{name: "exact limit", rows: 1, want: `{"columns":["id","data"],"rows":[{"data":{"base64":"AQI="},"id":1}],"truncated":false}`},
		{name: "above limit", rows: 2, want: `{"columns":["id","data"],"rows":[{"data":{"base64":"AQI="},"id":1}],"truncated":true}`},
		{name: "driver error", fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn := &sqlTestConnection{rows: test.rows, fail: test.fail}
			request := SQLExecution{
				open:      func() (*sql.DB, error) { return sql.OpenDB(conn), nil },
				statement: "SELECT $1", args: []any{int64(42)}, transactional: true, rowLimit: 1,
			}
			var output string
			err := request.Execute(context.Background(), func() error { return nil }, func(value any) error {
				output = string(value.(json.RawMessage))
				return nil
			})
			if test.fail {
				if err == nil || err.Error() != "SQL statement failed" || !conn.rolledBack || conn.committed {
					t.Fatalf("unexpected failure result: %v, commit=%v, rollback=%v", err, conn.committed, conn.rolledBack)
				}
			} else if err != nil || output != test.want || !conn.committed || conn.rolledBack {
				t.Fatalf("unexpected result: %s, %v, commit=%v, rollback=%v", output, err, conn.committed, conn.rolledBack)
			}
		})
	}
}

type sqlTestConnection struct {
	rows                  int64
	fail                  bool
	committed, rolledBack bool
}

func (conn *sqlTestConnection) Connect(context.Context) (driver.Conn, error) { return conn, nil }
func (conn *sqlTestConnection) Driver() driver.Driver                        { return nil }
func (conn *sqlTestConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (conn *sqlTestConnection) Close() error              { return nil }
func (conn *sqlTestConnection) Begin() (driver.Tx, error) { return conn, nil }
func (conn *sqlTestConnection) Commit() error             { conn.committed = true; return nil }
func (conn *sqlTestConnection) Rollback() error           { conn.rolledBack = true; return nil }

func (conn *sqlTestConnection) QueryContext(_ context.Context, statement string, args []driver.NamedValue) (driver.Rows, error) {
	if conn.fail || statement != "SELECT $1" || len(args) != 1 || args[0].Value != int64(42) {
		return nil, errors.New("driver error with sensitive details")
	}
	return &sqlTestRows{left: conn.rows}, nil
}

type sqlTestRows struct {
	left int64
}

func (*sqlTestRows) Columns() []string { return []string{"id", "data"} }
func (*sqlTestRows) Close() error      { return nil }
func (*sqlTestRows) ColumnTypeDatabaseTypeName(index int) string {
	return []string{"INT8", "BYTEA"}[index]
}
func (rows *sqlTestRows) Next(dest []driver.Value) error {
	if rows.left == 0 {
		return io.EOF
	}
	rows.left--
	dest[0], dest[1] = int64(1), []byte{1, 2}
	return nil
}
