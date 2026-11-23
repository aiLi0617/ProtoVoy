package main

import (
	"errors"
	"net/http"

	"protovoy.dev/protovoy/internal/execution"
)

type sqlEndpoint struct {
	target      string
	database    execution.SQLEndpoint
	passwordRef string
}

func sqlHandler(origin string, endpoint *sqlEndpoint) http.HandlerFunc {
	if endpoint == nil {
		endpoint = &sqlEndpoint{}
	}
	return brokerHandler(origin, "SQL", endpoint.target, func(input execution.SQLBridgeRequest) string {
		return input.ExpectedTarget
	}, func(input execution.SQLBridgeRequest) (brokerRunner, error) {
		if input.ExpectedPasswordRef != "" && input.ExpectedPasswordRef != endpoint.passwordRef {
			return nil, errors.New("SQL password reference does not match the configured database")
		}
		request, err := endpoint.database.Prepare(input.SQLRequest)
		return request.Execute, err
	}, func(input execution.SQLBridgeRequest) int { return input.TimeoutMS })
}
