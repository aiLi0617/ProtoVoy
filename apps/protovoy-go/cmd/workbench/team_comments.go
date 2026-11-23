package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/vivaui/viva/flex"
	"protovoy.dev/protovoy/internal/teamclient"
)

var (
	teamCommentsID     = tree.ID(page, "team-comments")
	teamCommentsHideID = tree.ID(page, "team-comments-hide")
	teamCommentBodyID  = tree.ID(page, "team-comment-body")
	teamCommentSaveID  = tree.ID(page, "team-comment-save")
	teamCommentResetID = tree.ID(page, "team-comment-reset")
	teamCommentPrevID  = tree.ID(page, "team-comment-prev")
	teamCommentNextID  = tree.ID(page, "team-comment-next")
)

type teamComments struct {
	workspace, parent, editing string
	body                       flex.Field
	items                      []teamclient.Comment
	page                       int
}

type teamCommentResult struct {
	workspace string
	items     []teamclient.Comment
	item      teamclient.Comment
	clear     bool
	watch     *teamWatch
	seen      uint64
}

func (w *workbench) drawTeamComments(f *flex.Frame, id string) {
	if id == "" {
		return
	}
	panel := &w.team.comments
	client := w.team.client
	if f.Button(teamCommentsID, "Load workspace comments").Clicked() {
		watch := w.team.watch
		var seen uint64
		if watch != nil && watch.id == id {
			w.mu.Lock()
			seen = watch.comments
			w.mu.Unlock()
		}
		w.runTeamAction("Loading workspace comments…", func(ctx context.Context) teamActionResult {
			items, err := client.Comments(ctx, id)
			return teamActionResult{
				comments: &teamCommentResult{workspace: id, items: items, watch: watch, seen: seen},
				status:   "Workspace comments loaded", err: err,
			}
		})
		return
	}
	if panel.workspace != id {
		return
	}
	if f.Button(teamCommentsHideID, "Hide workspace comments").Clicked() {
		panel.workspace = ""
		panel.resetDraft()
		return
	}
	label := "Post comment"
	switch {
	case panel.editing != "":
		label = "Save comment edit"
	case panel.parent != "":
		label = "Post reply"
	}
	f.Field(&panel.body, teamCommentBodyID, "Comment text", "Write a comment")
	if f.Button(teamCommentSaveID, label).Clicked() && strings.TrimSpace(panel.body.Text()) != "" {
		w.saveTeamComment(id)
		return
	}
	if (panel.editing != "" || panel.parent != "") && f.Button(teamCommentResetID, "Cancel comment edit or reply").Clicked() {
		panel.resetDraft()
	}
	const pageSize = 5
	start := panel.page * pageSize
	end := min(start+pageSize, len(panel.items))
	if end == 0 {
		f.Text("No workspace comments")
		return
	}
	f.Text(fmt.Sprintf("Comments %d–%d of %d", start+1, end, len(panel.items)))
	for _, item := range panel.items[start:end] {
		if w.drawTeamComment(f, id, item) {
			return
		}
	}
	if start > 0 && f.Button(teamCommentPrevID, "Previous comments").Clicked() {
		panel.page--
	}
	if end < len(panel.items) && f.Button(teamCommentNextID, "Next comments").Clicked() {
		panel.page++
	}
}

func (w *workbench) drawTeamComment(f *flex.Frame, workspace string, item teamclient.Comment) bool {
	panel, client := &w.team.comments, w.team.client
	author := item.ActorName
	if item.ParentID != nil {
		for _, parent := range panel.items {
			if parent.ID == *item.ParentID {
				author += " · Reply to " + parent.ActorName
				break
			}
		}
	}
	if item.ResolvedAt != nil {
		author += " · Resolved"
	}
	f.Wrap(author).Muted()
	f.Wrap(item.Body)
	if f.Button(tree.ID(page, "team-comment-reply-"+item.ID), "Reply").Clicked() {
		panel.resetDraft()
		panel.parent = item.ID
	}
	admin := client.Role == "OWNER" || client.Role == "ADMIN"
	if (admin || item.ActorID == client.UserID) && f.Button(tree.ID(page, "team-comment-edit-"+item.ID), "Edit comment").Clicked() {
		panel.resetDraft()
		panel.editing = item.ID
		panel.body.SetText(item.Body)
	}
	if admin || client.Role == "EDITOR" {
		resolve := item.ResolvedAt == nil
		label := "Resolve comment"
		if !resolve {
			label = "Reopen comment"
		}
		if f.Button(tree.ID(page, "team-comment-resolve-"+item.ID), label).Clicked() {
			w.runTeamAction("Updating comment…", func(ctx context.Context) teamActionResult {
				value, err := client.ResolveComment(ctx, workspace, item.ID, resolve)
				return teamActionResult{comments: &teamCommentResult{workspace: workspace, item: value}, status: "Comment updated", err: err}
			})
			return true
		}
	}
	return false
}

func (w *workbench) saveTeamComment(workspace string) {
	panel, client := &w.team.comments, w.team.client
	body, parent, editing := panel.body.Text(), panel.parent, panel.editing
	w.runTeamAction("Saving comment…", func(ctx context.Context) teamActionResult {
		var value teamclient.Comment
		var err error
		if editing != "" {
			value, err = client.EditComment(ctx, workspace, editing, body)
		} else {
			value, err = client.PostComment(ctx, workspace, parent, body)
		}
		return teamActionResult{comments: &teamCommentResult{workspace: workspace, item: value, clear: true}, status: "Comment saved", err: err}
	})
}

func (panel *teamComments) resetDraft() {
	panel.parent, panel.editing = "", ""
	panel.body.SetText("")
}

func (panel *teamComments) apply(result teamCommentResult) {
	if result.items != nil {
		if panel.workspace != result.workspace {
			panel.resetDraft()
		}
		panel.workspace, panel.items, panel.page = result.workspace, result.items, 0
		return
	}
	if panel.workspace != result.workspace {
		return
	}
	if result.clear {
		panel.resetDraft()
	}
	for index := range panel.items {
		if panel.items[index].ID == result.item.ID {
			panel.items[index] = result.item
			return
		}
	}
	panel.items = append(panel.items, result.item)
}
