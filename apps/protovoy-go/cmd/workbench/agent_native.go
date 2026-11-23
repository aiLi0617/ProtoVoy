//go:build !js

package main

import "net/http"

var agentHTTP = &http.Client{
	Transport: &http.Transport{},
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func doAgentRequest(request *http.Request) (*http.Response, error) {
	return agentHTTP.Do(request)
}
