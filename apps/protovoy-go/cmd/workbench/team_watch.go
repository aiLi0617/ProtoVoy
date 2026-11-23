package main

import (
	"context"
	"fmt"

	"github.com/vivaui/viva/flex"
	"protovoy.dev/protovoy/internal/teamclient"
)

var (
	teamWatchID     = tree.ID(page, "team-watch")
	teamWatchStopID = tree.ID(page, "team-watch-stop")
)

type teamWatch struct {
	id     string
	cancel context.CancelFunc
	// The reader goroutine updates these fields under workbench.mu.
	connected bool
	revision  int64
	comments  uint64
	seen      uint64
	err       error
}

func (w *workbench) drawTeamWatch(f *flex.Frame, id string) {
	watch := w.team.watch
	if watch == nil {
		if id != "" && f.Button(teamWatchID, "Watch team updates").Clicked() {
			w.startTeamWatch(id)
		}
		return
	}
	w.mu.Lock()
	connected, revision, err := watch.connected, watch.revision, watch.err
	hasComments := watch.comments > watch.seen
	w.mu.Unlock()
	if err != nil {
		f.Wrap(err.Error() + ". Start watching again and load the current revision to catch up.")
		if id != "" && f.Button(teamWatchID, "Watch team updates").Clicked() {
			w.startTeamWatch(id)
		}
		return
	}
	if f.Button(teamWatchStopID, "Stop watching team updates").Clicked() {
		w.stopTeamWatch()
		return
	}
	if !connected {
		f.Wrap("Connecting to team updates…")
		return
	}
	f.Wrap("Watching team updates for " + watch.id)
	if watch.id == id && revision > 0 && (watch.id != w.team.workspaceID || revision > w.team.revision) {
		f.Wrap(fmt.Sprintf("Team revision %d is available. Load it to update your project.", revision))
	}
	if watch.id == id && hasComments {
		f.Wrap("Workspace comments changed. Load comments to see the discussion.")
	}
}

func (w *workbench) startTeamWatch(id string) {
	w.stopTeamWatch()
	// Keep the watch's session independent of concurrent sign-out.
	client := *w.team.client
	ctx, cancel := context.WithCancel(context.Background())
	watch := &teamWatch{id: id, cancel: cancel}
	w.team.watch = watch
	go func() {
		defer cancel()
		update := func(change func()) {
			w.mu.Lock()
			change()
			w.mu.Unlock()
			w.invalidator.Invalidate()
		}
		err := client.Watch(ctx, func() {
			update(func() { watch.connected = true })
		}, func(event teamclient.WorkspaceEvent) {
			if event.WorkspaceID == id {
				update(func() {
					if event.CommentID != "" {
						watch.comments++
					} else {
						watch.revision = max(watch.revision, event.Revision)
					}
				})
			}
		})
		if ctx.Err() == nil {
			update(func() { watch.err = err })
		}
	}()
}

func (w *workbench) stopTeamWatch() {
	if watch := w.team.watch; watch != nil {
		watch.cancel()
		w.team.watch = nil
	}
}
