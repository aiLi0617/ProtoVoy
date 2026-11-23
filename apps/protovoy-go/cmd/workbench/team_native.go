//go:build !js

package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/vivaui/viva/app"
	"protovoy.dev/protovoy/internal/teamclient"
)

func teamServerOrigin() string { return "" }

func (w *workbench) startTeamSSO() {
	w.team.password.SetText("")
	client, err := teamclient.New(strings.TrimSpace(w.team.origin.Text()), nil)
	if err != nil {
		w.team.status = err.Error()
		return
	}
	w.runTeamActionTimeout("Complete team sign-in in your browser…", 3*time.Minute, func(ctx context.Context) teamActionResult {
		err := client.CompleteDesktopSSO(ctx, app.OpenURL)
		if errors.Is(err, context.Canceled) {
			return teamActionResult{status: "SSO sign-in cancelled"}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return teamActionResult{status: "SSO sign-in timed out"}
		}
		return teamActionResult{client: client, status: "Signed in with SSO", err: err}
	})
}
func (*workbench) resumeTeamSSO() {}
