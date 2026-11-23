//go:build !js

package execution

import (
	"errors"
	"net/url"
	"os"
)

const (
	SQLPasswordRefEnv = "PROTOVOY_SQL_PASSWORD_REF"
	SQLTargetEnv      = "PROTOVOY_SQL_TARGET"
	SQLUsernameEnv    = "PROTOVOY_SQL_USERNAME"
)

// SQLPasswordBinding binds a named credential to a single verified endpoint.
type SQLPasswordBinding struct {
	reference, target, username string
	endpoint                    SQLEndpoint
}

func LoadSQLPasswordBinding() (SQLPasswordBinding, error) {
	return NewSQLPasswordBinding(os.Getenv(SQLPasswordRefEnv), os.Getenv(SQLTargetEnv), os.Getenv(SQLUsernameEnv), os.Getenv(SQLPasswordEnv))
}

func NewSQLPasswordBinding(reference, target, username, password string) (SQLPasswordBinding, error) {
	if reference == "" {
		return SQLPasswordBinding{}, nil
	}
	if password == "" {
		return SQLPasswordBinding{}, errors.New("SQL password binding requires a valid reference, username, and password")
	}
	endpoint, err := sqlPasswordEndpoint(reference, target, username, password)
	if err != nil {
		return SQLPasswordBinding{}, err
	}
	return SQLPasswordBinding{reference, target, username, endpoint}, nil
}

func sqlPasswordEndpoint(reference, target, username, password string) (SQLEndpoint, error) {
	if reference == "" || !ValidPasswordReference(reference) || username == "" {
		return SQLEndpoint{}, errors.New("SQL password binding requires a valid reference and username")
	}
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" && parsed.Scheme != "mysql") ||
		parsed.Query().Get("sslmode") == "disable" {
		return SQLEndpoint{}, errors.New("SQL password binding requires a PostgreSQL or MySQL target with verified TLS")
	}
	return NewSQLEndpoint(target, username, password)
}

func ResolveSQLPasswordBinding(reference, target, username string, resolve func(string) (string, error)) (SQLPasswordBinding, error) {
	if _, err := sqlPasswordEndpoint(reference, target, username, ""); err != nil {
		return SQLPasswordBinding{}, err
	}
	if resolve == nil {
		return SQLPasswordBinding{}, errors.New("SQL password reference could not be resolved")
	}
	password, err := resolve(reference)
	if err != nil || password == "" {
		return SQLPasswordBinding{}, errors.New("SQL password reference could not be resolved")
	}
	return NewSQLPasswordBinding(reference, target, username, password)
}

func (binding SQLPasswordBinding) Resolve(target string, credentials SQLCredentials) (SQLEndpoint, error) {
	if credentials.PasswordRef == "" {
		return NewSQLEndpoint(target, credentials.Username, credentials.Password)
	}
	if err := binding.checkReference(target, credentials); err != nil {
		return SQLEndpoint{}, err
	}
	return binding.endpoint, nil
}

func ResolveSQLWithPasswordResolver(target string, credentials SQLCredentials, resolve func(string) (string, error)) (SQLEndpoint, error) {
	if credentials.PasswordRef == "" {
		return NewSQLEndpoint(target, credentials.Username, credentials.Password)
	}
	configured := SQLPasswordBinding{
		reference: os.Getenv(SQLPasswordRefEnv),
		target:    os.Getenv(SQLTargetEnv),
		username:  os.Getenv(SQLUsernameEnv),
	}
	if err := configured.checkReference(target, credentials); err != nil {
		return SQLEndpoint{}, err
	}
	binding, err := ResolveSQLPasswordBinding(configured.reference, configured.target, configured.username, resolve)
	if err != nil {
		return SQLEndpoint{}, err
	}
	return binding.endpoint, nil
}

func (binding SQLPasswordBinding) checkReference(target string, credentials SQLCredentials) error {
	if credentials.Password != "" {
		return errors.New("use a SQL password reference or a session password")
	}
	if credentials.PasswordRef != binding.reference || target != binding.target ||
		(credentials.Username != "" && credentials.Username != binding.username) {
		return errors.New("SQL password reference is unavailable for this database and username")
	}
	return nil
}
