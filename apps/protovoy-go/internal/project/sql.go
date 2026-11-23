package project

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

type SQLSettings struct {
	Username      string `json:"username,omitempty"`
	PasswordRef   string `json:"passwordRef,omitempty"`
	Parameters    string `json:"parameters,omitempty"`
	Transactional bool   `json:"transactional,omitempty"`
	RowLimit      int    `json:"rowLimit"`
	TimeoutMS     int    `json:"timeoutMs,omitempty"`
}

func (settings SQLSettings) Request(statement string) (execution.SQLRequest, error) {
	if !execution.ValidRequestTimeout(settings.TimeoutMS) {
		return execution.SQLRequest{}, errors.New("SQL timeout must be at most five minutes")
	}
	if !execution.ValidPasswordReference(settings.PasswordRef) {
		return execution.SQLRequest{}, errors.New("enter a valid SQL password reference")
	}
	if strings.TrimSpace(statement) == "" || len(statement) > 1<<20 {
		return execution.SQLRequest{}, errors.New("enter a SQL statement within 1 MiB")
	}
	if settings.RowLimit < 1 || settings.RowLimit > 10_000 {
		return execution.SQLRequest{}, errors.New("enter a SQL row limit from 1 to 10,000")
	}
	parameters := []json.RawMessage{}
	if input := strings.TrimSpace(settings.Parameters); input != "" &&
		(json.Unmarshal([]byte(input), &parameters) != nil || parameters == nil) {
		return execution.SQLRequest{}, errors.New("SQL parameters must be a JSON array")
	}
	return execution.SQLRequest{
		Statement: statement, Parameters: parameters,
		Transactional: settings.Transactional, RowLimit: &settings.RowLimit,
	}, nil
}

var sqlPasswordLiteral = regexp.MustCompile(`(?i)\b(?:password|identified\s+by)\s+'`)

func (settings SQLSettings) Validate(target, statement string) error {
	if settings.containsSensitiveData(target, statement) || (Request{URL: target, Body: statement}).containsSensitiveData() {
		return errors.New("remove possible credentials from the SQL request")
	}
	_, err := settings.Request(statement)
	return err
}

func (settings SQLSettings) containsSensitiveData(target, statement string) bool {
	return strings.Contains(target, "@") || sensitiveBody.MatchString(settings.Parameters) ||
		sqlPasswordLiteral.MatchString(statement) || hasPrivateKey(statement) || hasPrivateKey(settings.Parameters)
}
