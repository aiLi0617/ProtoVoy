package main

import (
	"errors"
	"net/http"

	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

func validateHostedScripts(request execution.PreviewRequest) error {
	if request.EventStream && (len(request.PreScripts) != 0 || len(request.PostScripts) != 0) {
		return errors.New("hosted request scripts require HTTP without streaming")
	}
	return (project.Request{PreScripts: request.PreScripts, PostScripts: request.PostScripts}).ValidateScripts()
}

func validHostedHTTPMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return true
	}
	return false
}
