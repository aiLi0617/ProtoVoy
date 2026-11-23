//go:build !js

package main

import (
	"os"
	"testing"

	"github.com/vivaui/viva/headless"
	"github.com/vivaui/viva/ui"
)

func TestDesktopTeamShowsSSOAction(t *testing.T) {
	w := newWorkbench()
	w.team.open = true
	config := w.config()
	config.Width, config.Height = 900, 650
	session, err := headless.Open(config, &ui.Canvas{}, headless.Options{Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Expect("text", "Sign in with SSO"); err != nil {
		t.Fatal(err)
	}
	if destination := os.Getenv("PROTOVOY_TEST_SSO_SCREENSHOT"); destination != "" {
		if err := session.Screenshot(destination); err != nil {
			t.Fatal(err)
		}
	}
}
