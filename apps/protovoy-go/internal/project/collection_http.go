package project

import (
	"encoding/json"
	"errors"
	"strings"

	"protovoy.dev/protovoy/internal/execution"
)

// CollectionHTTPProtocol identifies requests that finish with an HTTP response.
func (request Request) CollectionHTTPProtocol() string {
	switch {
	case request.protocolCount() == 0:
		return "http"
	case request.RPC != nil && request.RPC.Valid():
		return request.RPC.Protocol
	case request.GraphQL != nil && !request.IsGraphQLSubscription():
		return "graphql"
	default:
		return ""
	}
}

func (request Request) IsGraphQLSubscription() bool {
	return request.GraphQL != nil && (request.GraphQL.Subscription || startsWithSubscription(request.GraphQL.Query))
}

func (request Request) PrepareCollectionHTTP(collectionVariables map[string]string) (execution.HTTPRequest, error) {
	headers, err := execution.ParseHeaders(request.HeaderText())
	if err != nil {
		return execution.HTTPRequest{}, err
	}
	scopes := []map[string]string{collectionVariables, request.Variables}
	target := request.URL
	var payload execution.HTTPPayload
	switch request.CollectionHTTPProtocol() {
	case "http":
		payload = execution.HTTPPayload{
			Method: request.Method, Headers: headers, Body: request.Body, BodyEncoding: request.BodyEncoding,
			Multipart: request.Multipart, TimeoutMS: request.HTTPTimeoutMS, FollowRedirects: request.FollowRedirects,
		}
		target, payload, err = payload.ResolveVariables(target, scopes...)
	case "graphql":
		target, payload, err = request.GraphQL.Prepare(target, headers, scopes...)
	case "jsonrpc":
		target, payload, err = execution.PrepareJSONRPC(target, request.RPC.Method,
			json.RawMessage(strings.TrimSpace(request.Body)), json.RawMessage(request.RPC.ID), headers, scopes...)
		payload.TimeoutMS = request.RPC.TimeoutMS
	case "soap":
		payload, err = execution.SOAPPayload(request.RPC.Version, request.RPC.Action, request.Body, headers)
		if err == nil {
			target, payload, err = payload.ResolveVariables(target, scopes...)
		}
		payload.TimeoutMS = request.RPC.TimeoutMS
	default:
		return execution.HTTPRequest{}, errors.New("unsupported collection protocol")
	}
	if err != nil {
		return execution.HTTPRequest{}, err
	}
	payload, err = payload.EncodeBody()
	if err != nil {
		return execution.HTTPRequest{}, err
	}
	return payload.Request(target), nil
}
