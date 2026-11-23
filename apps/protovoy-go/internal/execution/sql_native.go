//go:build !js

package execution

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const SQLPasswordEnv = "PROTOVOY_SQL_PASSWORD"

type SQLEndpoint struct {
	open func() (*sql.DB, error)
}

type SQLExecution struct {
	open          func() (*sql.DB, error)
	statement     string
	args          []any
	transactional bool
	rowLimit      int
}

func NewSQLEndpoint(target, username, password string) (SQLEndpoint, error) {
	open, err := sqlConnector(target, username, password)
	return SQLEndpoint{open: open}, err
}

func (endpoint SQLEndpoint) Prepare(request SQLRequest) (SQLExecution, error) {
	statement := request.Statement
	args := make([]any, len(request.Parameters))
	for i, parameter := range request.Parameters {
		decoder := json.NewDecoder(strings.NewReader(string(parameter)))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) != nil {
			return SQLExecution{}, errors.New("invalid SQL parameter")
		}
		switch value := value.(type) {
		case json.Number:
			if !strings.ContainsAny(value.String(), ".eE") {
				number, err := value.Int64()
				if err != nil {
					return SQLExecution{}, errors.New("SQL integer parameter exceeds signed 64-bit range")
				}
				args[i] = number
			} else {
				number, err := value.Float64()
				if err != nil {
					return SQLExecution{}, errors.New("invalid SQL decimal parameter")
				}
				args[i] = number
			}
		case map[string]any, []any:
			data, err := json.Marshal(value)
			if err != nil || len(data) > 1<<20 {
				return SQLExecution{}, errors.New("invalid SQL JSON parameter")
			}
			args[i] = string(data)
		default:
			args[i] = value
		}
	}
	if strings.TrimSpace(statement) == "" || len(statement) > 1<<20 {
		return SQLExecution{}, errors.New("enter a SQL statement within 1 MiB")
	}
	rowLimit := 500
	if request.RowLimit != nil {
		rowLimit = max(1, min(*request.RowLimit, 10_000))
	}
	return SQLExecution{endpoint.open, statement, args, request.Transactional, rowLimit}, nil
}

func sqlConnector(target, username, password string) (func() (*sql.DB, error), error) {
	if strings.HasPrefix(target, "sqlite:") {
		return sqliteConnector(target, username, password)
	}
	invalid := errors.New("enter a PostgreSQL, MySQL or SQLite URL and supply credentials separately")
	parsed, err := url.Parse(target)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" || parsed.Path == "" ||
		strings.Contains(target, "#") || strings.ContainsAny(parsed.Hostname(), ",/\\") ||
		!strings.HasPrefix(parsed.Path, "/") || len(parsed.Path) < 2 {
		return nil, invalid
	}
	port := parsed.Port()
	if port != "" {
		if number, err := strconv.ParseUint(port, 10, 16); err != nil || number == 0 {
			return nil, invalid
		}
	}
	options, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return nil, invalid
	}
	switch parsed.Scheme {
	case "postgres", "postgresql":
		mode := options.Get("sslmode")
		if mode == "" {
			mode = "verify-full"
		}
		if len(options) > 1 || len(options) == 1 && len(options["sslmode"]) != 1 ||
			(mode != "verify-full" && mode != "disable") {
			return nil, errors.New("PostgreSQL supports sslmode=verify-full or explicit sslmode=disable")
		}
		if port == "" {
			port = "5432"
		}
		// Initialize pgx with a fixed DSN, then assign request values as fields
		// so untrusted input never reaches its connection-string parser.
		config, err := pgx.ParseConfig("postgres://postgres:unused@localhost:5432/postgres?" +
			"sslmode=disable&sslrootcert=&sslcert=&sslkey=&sslnegotiation=postgres&target_session_attrs=any")
		if err != nil {
			return nil, invalid
		}
		portNumber, _ := strconv.ParseUint(port, 10, 16)
		config.Host, config.Port, config.Database = parsed.Hostname(), uint16(portNumber), parsed.Path[1:]
		if username != "" {
			config.User = username
		}
		config.Password = password
		config.Fallbacks, config.RuntimeParams = nil, nil
		if mode == "verify-full" {
			config.TLSConfig = &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12}
		}
		return func() (*sql.DB, error) { return stdlib.OpenDB(*config), nil }, nil
	case "mysql":
		mode := options.Get("ssl-mode")
		if len(options) != 0 && (len(options) != 1 || len(options["ssl-mode"]) != 1 ||
			!strings.EqualFold(mode, "VERIFY_IDENTITY") && !strings.EqualFold(mode, "REQUIRED") && !strings.EqualFold(mode, "VERIFY_CA")) {
			return nil, errors.New("MySQL requires verified TLS")
		}
		if port == "" {
			port = "3306"
		}
		config := mysql.NewConfig()
		config.User, config.Passwd = username, password
		config.Net, config.Addr = "tcp", net.JoinHostPort(parsed.Hostname(), port)
		config.DBName, config.TLSConfig, config.ParseTime = parsed.Path[1:], "true", true
		connector, err := mysql.NewConnector(config)
		if err != nil {
			return nil, invalid
		}
		return func() (*sql.DB, error) { return sql.OpenDB(connector), nil }, nil
	default:
		return nil, invalid
	}
}

func (request SQLExecution) Execute(ctx context.Context, connected func() error, emit func(any) error) error {
	db, err := request.open()
	if err != nil {
		return errors.New("could not open SQL connection")
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return errors.New("could not connect to SQL endpoint")
	}
	defer conn.Close()
	if err := connected(); err != nil {
		return err
	}
	var executor sqlExecutor = conn
	var tx *sql.Tx
	if request.transactional {
		tx, err = conn.BeginTx(ctx, nil)
		if err != nil {
			return errors.New("could not start SQL transaction")
		}
		defer tx.Rollback()
		executor = tx
	}
	result, err := request.run(ctx, executor)
	if err != nil {
		return err
	}
	if tx != nil && tx.Commit() != nil {
		return errors.New("could not commit SQL transaction")
	}
	return emit(result)
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (request SQLExecution) run(ctx context.Context, executor sqlExecutor) (json.RawMessage, error) {
	failure := errors.New("SQL statement failed")
	words := strings.Fields(request.statement)
	switch strings.ToUpper(words[0]) {
	case "SELECT", "WITH", "SHOW", "EXPLAIN", "PRAGMA", "DESCRIBE", "DESC", "VALUES", "TABLE":
		rows, err := executor.QueryContext(ctx, request.statement, request.args...)
		if err != nil {
			return nil, failure
		}
		defer rows.Close()
		return collectSQLRows(rows, request.rowLimit)
	default:
		result, err := executor.ExecContext(ctx, request.statement, request.args...)
		if err != nil {
			return nil, failure
		}
		affected, _ := result.RowsAffected()
		insertID, _ := result.LastInsertId()
		return json.Marshal(map[string]int64{"rowsAffected": affected, "lastInsertId": insertID})
	}
}

func collectSQLRows(rows *sql.Rows, limit int) (json.RawMessage, error) {
	columns, err := rows.ColumnTypes()
	if err != nil {
		return nil, errors.New("could not read SQL columns")
	}
	result := struct {
		Columns   []string          `json:"columns"`
		Rows      []json.RawMessage `json:"rows"`
		Truncated bool              `json:"truncated"`
	}{Columns: make([]string, len(columns)), Rows: []json.RawMessage{}}
	cells := make([]any, len(columns))
	destinations := make([]any, len(columns))
	for i, column := range columns {
		result.Columns[i] = column.Name()
		destinations[i] = &cells[i]
	}
	size := 0
	for rows.Next() {
		if len(result.Rows) == limit {
			result.Truncated = true
			break
		}
		if rows.Scan(destinations...) != nil {
			return nil, errors.New("could not read SQL row")
		}
		record := make(map[string]any, len(columns))
		for i, column := range columns {
			record[column.Name()] = sqlCell(cells[i], column.DatabaseTypeName())
		}
		data, err := json.Marshal(record)
		if err != nil || size+len(data) > 15<<20 {
			return nil, errors.New("SQL result exceeds its response limit")
		}
		size += len(data)
		result.Rows = append(result.Rows, data)
	}
	if rows.Close() != nil || rows.Err() != nil {
		return nil, errors.New("could not finish reading SQL results")
	}
	data, err := json.Marshal(result)
	if err != nil || len(data) >= 16<<20 {
		return nil, errors.New("SQL result exceeds its response limit")
	}
	return data, nil
}

func sqlCell(cell any, typeName string) any {
	switch value := cell.(type) {
	case []byte:
		if typeName == "" || strings.Contains(typeName, "BINARY") || strings.Contains(typeName, "BLOB") || typeName == "BYTEA" || typeName == "BIT" {
			return map[string]string{"base64": base64.StdEncoding.EncodeToString(value)}
		}
		return string(value)
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return strconv.FormatFloat(value, 'g', -1, 64)
		}
	}
	return cell
}
