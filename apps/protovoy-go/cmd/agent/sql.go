package main

import (
	"errors"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

func (agent *agentService) prepareSQL(envelope requestEnvelope) (execution.SQLExecution, error) {
	var input struct {
		Type  string `json:"type"`
		Value struct {
			execution.SQLRequest
			Username    string `json:"username"`
			Password    string `json:"password"`
			PasswordRef string `json:"passwordRef"`
			Literal     bool   `json:"literal"`
		} `json:"value"`
	}
	if !decodeExecutionPayload(envelope.Payload, &input) || input.Type != "raw" {
		return execution.SQLExecution{}, errors.New("invalid SQL execution payload")
	}
	request := execution.SQLBridgeRequest{ExpectedTarget: envelope.Target, SQLRequest: input.Value.SQLRequest}
	if !input.Value.Literal {
		var err error
		request, err = request.ResolveVariables(envelope.Variables)
		if err != nil {
			return execution.SQLExecution{}, err
		}
	}
	target := request.ExpectedTarget
	if strings.HasPrefix(target, "sqlite:") && target != execution.SQLiteMemoryTarget && target != agent.sqliteTarget {
		return execution.SQLExecution{}, errors.New("SQLite file target is not configured for this Local Agent")
	}
	endpoint, err := agent.sqlPasswords.Resolve(target, execution.SQLCredentials{
		Username: input.Value.Username, Password: input.Value.Password, PasswordRef: input.Value.PasswordRef,
	})
	if err != nil {
		return execution.SQLExecution{}, err
	}
	return endpoint.Prepare(request.SQLRequest)
}
