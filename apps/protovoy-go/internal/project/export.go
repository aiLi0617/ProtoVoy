package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var sensitiveKey = regexp.MustCompile(`(?i)(^|[-_])(authorization|api[-_]?key|token|secret|password|passwd|cookie|set-cookie)($|[-_])`)
var sensitiveBody = regexp.MustCompile(`(?i)[\w-]*(token|secret|password|api[-_]?key)[\w-]*["']?\s*[:=]`)

// Encode keeps HTTP-only collections compatible with the legacy importer.
func Encode(value Project) ([]byte, error) {
	if len(value.Requests) > maxRequests {
		return nil, errors.New("project exceeds 10,000 requests")
	}
	if err := validateEmptyFolders(value.EmptyFolders); err != nil {
		return nil, err
	}
	multi := len(value.CollectionVariables) != 0 || len(value.EmptyFolders) != 0
	if sensitiveVariables(value.CollectionVariables) {
		return nil, errors.New("remove possible credentials from collection variables before exporting")
	}
	for _, request := range value.Requests {
		multi = multi || request.HasScripts() || request.HTTPSecretReference != "" || request.HTTPSecretHeader != "" || request.protocolCount() != 0 || request.FollowRedirects ||
			request.BodyEncoding != "" || request.BodySource != "" || len(request.Multipart) != 0 ||
			request.HTTPTimeoutMS != 0 || request.ExpectedHTTPStatus != 0 || request.ResponseBodyContains != ""
	}
	version, format := 1, legacyProjectFormat
	if multi {
		version, format = 2, multiProjectFormat
	}
	requests := make([]*Request, 0, len(value.Requests))
	for index, request := range value.Requests {
		request.LocalID = ""
		if request.containsSensitiveData() {
			return nil, errors.New("remove possible credentials from the project before exporting")
		}
		if err := validatePortableRequest(&request, multi); err != nil {
			return nil, fmt.Errorf("request %d: %w", index+1, err)
		}
		if !multi && request.Headers == nil {
			request.Headers = map[string]string{}
		}
		requests = append(requests, &request)
	}
	document := projectFile{
		Format: format, Version: &version, Name: value.Name, Requests: requests, EmptyFolders: value.EmptyFolders,
		ExportedAt: time.Now().UTC().Format(time.RFC3339Nano), CollectionVariables: value.CollectionVariables,
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, errors.New("could not encode project")
	}
	if len(data) > MaxBytes {
		return nil, errors.New("project exceeds 8 MiB")
	}
	return data, nil
}

func (request Request) containsSensitiveData() bool {
	for _, phase := range [][]string{request.PreScripts, request.PostScripts} {
		for _, source := range phase {
			if sensitiveBody.MatchString(source) || hasPrivateKey(source) {
				return true
			}
		}
	}
	if sensitiveBody.MatchString(request.ResponseBodyContains) || hasPrivateKey(request.ResponseBodyContains) {
		return true
	}
	if sensitiveMultipart(request.Multipart) || sensitiveBodySource(request.BodySource) {
		return true
	}
	if request.Kafka != nil && request.Kafka.containsSensitiveData(request.URL) {
		return true
	}
	if request.WebSocket != nil {
		for _, message := range request.WebSocket.Messages {
			if sensitiveBody.MatchString(message.Data) || hasPrivateKey(message.Data) {
				return true
			}
		}
	}
	if request.AMQP != nil && request.AMQP.containsSensitiveData(request.URL) {
		return true
	}
	if request.SQL != nil && request.SQL.containsSensitiveData(request.URL, request.Body) {
		return true
	}
	if request.MQTT != nil && request.MQTT.containsSensitiveData(request.URL) {
		return true
	}
	if request.Redis != nil && request.Redis.CheckPersistence(request.URL, request.Body) != nil {
		return true
	}
	if request.Socket != nil && (hasPrivateKey(request.Socket.CAPEM) || sensitiveBody.MatchString(request.Socket.Data) || hasPrivateKey(request.Socket.Data)) {
		return true
	}
	for key := range request.Headers {
		if sensitiveKey.MatchString(key) {
			return true
		}
	}
	if sensitiveVariables(request.Variables) {
		return true
	}
	if target, err := url.Parse(request.URL); err == nil {
		if target.User != nil {
			return true
		}
		for key := range target.Query() {
			if sensitiveKey.MatchString(key) {
				return true
			}
		}
	}
	return sensitiveHTTPBody(request.Body, request.BodyEncoding == "base64") || request.GraphQL != nil &&
		(sensitiveBody.MatchString(request.GraphQL.Query) || sensitiveBody.MatchString(request.GraphQL.Variables))
}

func hasPrivateKey(input string) bool {
	return strings.Contains(strings.ToUpper(input), "PRIVATE KEY")
}
