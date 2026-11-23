//go:build js && wasm

package main

import (
	"context"
	"errors"
	"syscall/js"

	"protovoy.dev/protovoy/internal/teamclient"
)

func teamServerOrigin() string {
	return js.Global().Get("location").Get("origin").String()
}

func (w *workbench) startTeamSSO() {
	origin := teamServerOrigin()
	client, err := teamclient.New(origin, nil)
	if err != nil {
		w.team.status = err.Error()
		return
	}
	w.runTeamAction("Checking team SSO…", func(ctx context.Context) teamActionResult {
		enabled, err := client.SSOAvailable(ctx)
		if err == nil && !enabled {
			err = errors.New("team SSO is disabled")
		}
		if err == nil {
			js.Global().Get("location").Call("assign", origin+"/api/team/v1/auth/sso/login")
		}
		return teamActionResult{status: "Opening identity provider…", err: err}
	})
}

func (w *workbench) resumeTeamSSO() {
	location := js.Global().Get("location")
	if location.Get("hash").String() != "#team-sso" {
		return
	}
	js.Global().Get("history").Call("replaceState", js.Null(), "", location.Get("pathname").String()+location.Get("search").String())
	w.team.open = true
	client, err := teamclient.New(teamServerOrigin(), nil)
	if err != nil {
		w.team.status = err.Error()
		return
	}
	w.runTeamAction("Completing team sign-in…", func(ctx context.Context) teamActionResult {
		return teamActionResult{client: client, status: "Signed in with SSO", err: client.CompleteSSO(ctx)}
	})
}
