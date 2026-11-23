//go:build !js

package execution

import (
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
)

const SQLiteMemoryTarget = "sqlite::memory:"

func sqliteConnector(target, username, password string) (func() (*sql.DB, error), error) {
	if username != "" || password != "" {
		return nil, errors.New("SQLite does not use a username or password")
	}
	if target == SQLiteMemoryTarget {
		return func() (*sql.DB, error) { return driver.Open(":memory:", restrictSQLite) }, nil
	}
	path, mode, err := sqliteFileSettings(target)
	if err != nil {
		return nil, err
	}
	dsn := "file:" + (&url.URL{Path: filepath.ToSlash(path)}).EscapedPath() + "?mode=" + mode
	return func() (*sql.DB, error) { return driver.Open(dsn, restrictSQLite) }, nil
}

// SQLiteFilePath returns the native path used by a file-backed SQLite target.
func SQLiteFilePath(target string) (string, error) {
	path, _, err := sqliteFileSettings(target)
	return path, err
}

func sqliteFileSettings(target string) (string, string, error) {
	invalid := errors.New("enter a local sqlite: file URL with optional mode=ro, mode=rw, or mode=rwc")
	path, query, _ := strings.Cut(strings.TrimPrefix(target, "sqlite:"), "?")
	path, err := url.PathUnescape(strings.TrimPrefix(path, "//"))
	if err != nil || path == "" || path == ":memory:" || strings.ContainsAny(path, "\x00\r\n#") || strings.Contains(target, "#") {
		return "", "", invalid
	}
	options, err := url.ParseQuery(query)
	if err != nil || len(options) > 1 || len(options) == 1 && len(options["mode"]) != 1 {
		return "", "", invalid
	}
	mode := options.Get("mode")
	switch mode {
	case "":
		mode = "rw"
	case "ro", "rw", "rwc":
	default:
		return "", "", invalid
	}
	// Convert file URLs to native paths before resolving relative names.
	if filepath.Separator == '\\' && strings.HasPrefix(path, "/") && len(path) > 3 && path[2] == ':' {
		path = path[1:]
	}
	path, err = filepath.Abs(filepath.FromSlash(path))
	if err != nil || strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") ||
		strings.Contains(strings.TrimPrefix(path, filepath.VolumeName(path)), ":") {
		return "", "", invalid
	}
	return path, mode, nil
}

func restrictSQLite(conn *sqlite3.Conn) error {
	if _, err := conn.Config(sqlite3.DBCONFIG_DEFENSIVE, true); err != nil {
		return err
	}
	if _, err := conn.Config(sqlite3.DBCONFIG_TRUSTED_SCHEMA, false); err != nil {
		return err
	}
	if err := conn.Exec("PRAGMA temp_store = MEMORY"); err != nil {
		return err
	}
	conn.Limit(sqlite3.LIMIT_LENGTH, 8<<20)
	return conn.SetAuthorizer(func(action sqlite3.AuthorizerActionCode, name, argument, _, _ string) sqlite3.AuthorizerReturnCode {
		switch action {
		case sqlite3.AUTH_ATTACH, sqlite3.AUTH_DETACH:
			return sqlite3.AUTH_DENY
		case sqlite3.AUTH_FUNCTION:
			switch strings.ToLower(argument) {
			case "load_extension", "readfile", "writefile":
				return sqlite3.AUTH_DENY
			}
		case sqlite3.AUTH_PRAGMA:
			switch strings.ToLower(name) {
			case "table_info", "table_xinfo", "table_list", "index_list", "index_info", "index_xinfo",
				"foreign_key_list", "database_list", "quick_check", "integrity_check":
				return sqlite3.AUTH_OK
			case "foreign_keys", "user_version", "application_id", "schema_version", "journal_mode", "page_count", "query_only":
				if argument == "" {
					return sqlite3.AUTH_OK
				}
			}
			return sqlite3.AUTH_DENY
		}
		return sqlite3.AUTH_OK
	})
}
