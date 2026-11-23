package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/vivaui/viva/flex"
	"protovoy.dev/protovoy/internal/project"
	"protovoy.dev/protovoy/internal/teamclient"
)

var (
	teamPanelID    = tree.ID(page, "team-workspace")
	teamOriginID   = tree.ID(page, "team-origin")
	teamEmailID    = tree.ID(page, "team-email")
	teamPasswordID = tree.ID(page, "team-password")
	teamID         = tree.ID(page, "team-workspace-id")
	teamLoginID    = tree.ID(page, "team-login")
	teamSSOID      = tree.ID(page, "team-sso")
	teamLogoutID   = tree.ID(page, "team-logout")
	teamPullID     = tree.ID(page, "team-pull")
	teamPushID     = tree.ID(page, "team-push")
)

type teamPanel struct {
	open                                    bool
	origin, email, password, id             flex.Field
	client                                  *teamclient.Client
	workspaceID, status                     string
	revision                                int64
	pending                                 *teamActionResult // protected by workbench.mu
	members                                 []teamclient.Member
	memberEmail, memberName, memberPassword flex.Field
	memberRole                              flex.SelectState
	memberRoleIndex                         int
	audit                                   []teamclient.AuditEvent
	auditPage                               int
	watch                                   *teamWatch
	comments                                teamComments
}

type teamActionResult struct {
	client    *teamclient.Client
	id        string
	revision  int64
	project   *project.Project
	signature workspaceSignature
	status    string
	err       error
	members   []teamclient.Member
	member    *teamclient.Member
	audit     []teamclient.AuditEvent
	comments  *teamCommentResult
}

func (w *workbench) drawTeam(f *flex.Frame) {
	panel := &w.team
	f.Text("Team workspace")
	if w.cancel != nil {
		if f.Button(cancelID, "Cancel team operation").Clicked() {
			w.cancel()
		}
		f.Wrap(panel.status)
		return
	}
	if panel.client == nil {
		panel.password.Secure = true
		if teamServerOrigin() == "" {
			f.Field(&panel.origin, teamOriginID, "Team server origin", "https://workbench.example.com")
		}
		f.Field(&panel.email, teamEmailID, "Team email", "Email")
		f.Field(&panel.password, teamPasswordID, "Team password", "Password")
		if f.Button(teamLoginID, "Sign in to team").Clicked() {
			w.loginTeam()
		}
		if w.cancel == nil && f.Button(teamSSOID, "Sign in with SSO").Clicked() {
			w.startTeamSSO()
		}
	} else {
		f.Field(&panel.id, teamID, "Team workspace ID", "Workspace ID")
		id := strings.TrimSpace(panel.id.Text())
		if id != "" && f.Button(teamPullID, "Replace local project from team").Clicked() && w.canLeaveDraft() {
			w.pullTeam(id)
		}
		if id != "" && w.collection.project != nil && f.Button(teamPushID, "Publish saved project").Clicked() {
			w.pushTeam(id)
		}
		if f.Button(teamLogoutID, "Sign out of team").Clicked() {
			w.stopTeamWatch()
			client := panel.client
			w.runTeamAction("Signing out…", func(ctx context.Context) teamActionResult {
				return teamActionResult{status: "Signed out", err: client.Logout(ctx)}
			})
		}
		if w.cancel == nil {
			w.drawTeamWatch(f, id)
			w.drawTeamComments(f, id)
			if w.cancel == nil {
				w.drawTeamAudit(f)
			}
			if w.cancel == nil {
				w.drawTeamMembers(f)
			}
		}
	}
	f.Wrap(panel.status)
}

func (w *workbench) loginTeam() {
	origin := teamServerOrigin()
	if origin == "" {
		origin = strings.TrimSpace(w.team.origin.Text())
	}
	client, err := teamclient.New(origin, nil)
	if err != nil {
		w.team.status = err.Error()
		return
	}
	email, password := w.team.email.Text(), w.team.password.Text()
	w.team.password.SetText("")
	w.runTeamAction("Signing in…", func(ctx context.Context) teamActionResult {
		return teamActionResult{client: client, status: "Signed in", err: client.Login(ctx, email, password)}
	})
}

func (w *workbench) pullTeam(id string) {
	client, signature := w.team.client, w.workspaceSignature()
	w.runTeamAction("Loading team project…", func(ctx context.Context) teamActionResult {
		value, revision, err := client.Load(ctx, id)
		result := teamActionResult{client: client, id: id, revision: revision, signature: signature, err: err, status: "Team workspace is empty"}
		if err == nil && revision > 0 {
			result.project = &value
			result.status = fmt.Sprintf("Loaded team revision %d", revision)
		}
		return result
	})
}

func (w *workbench) pushTeam(id string) {
	client := w.team.client
	var revision int64
	if w.team.workspaceID == id {
		revision = w.team.revision
	}
	value := *w.collection.project
	value.Requests = slices.Clone(value.Requests)
	value.CollectionVariables = maps.Clone(w.variables.scopes.Collection)
	w.runTeamAction("Publishing team project…", func(ctx context.Context) teamActionResult {
		revision, err := client.Save(ctx, id, revision, value)
		return teamActionResult{client: client, id: id, revision: revision, err: err, status: fmt.Sprintf("Published team revision %d", revision)}
	})
}

func (w *workbench) runTeamAction(status string, action func(context.Context) teamActionResult) {
	w.runTeamActionTimeout(status, 30*time.Second, action)
}

func (w *workbench) runTeamActionTimeout(status string, timeout time.Duration, action func(context.Context) teamActionResult) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	w.cancel, w.team.status = cancel, status
	go func() {
		defer cancel()
		result := action(ctx)
		w.mu.Lock()
		w.team.pending = &result
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) finishTeamAction() {
	w.mu.Lock()
	result := w.team.pending
	w.team.pending = nil
	w.mu.Unlock()
	if result == nil {
		return
	}
	w.cancel = nil
	panel := &w.team
	if result.err != nil {
		var conflict *teamclient.Conflict
		if errors.As(result.err, &conflict) {
			panel.status = fmt.Sprintf("Team revision is now %d. Load it before publishing again.", conflict.Revision)
		} else {
			panel.status = result.err.Error()
		}
		return
	}
	if result.members != nil {
		panel.members = result.members
		for _, member := range panel.members {
			if member.ID == panel.client.UserID {
				panel.client.Role = member.Role
			}
		}
		if panel.client.Role != "OWNER" && panel.client.Role != "ADMIN" {
			panel.audit = nil
		}
		panel.status = result.status
		return
	}
	if result.comments != nil {
		panel.comments.apply(*result.comments)
		if watch := result.comments.watch; watch != nil && watch == panel.watch {
			w.mu.Lock()
			watch.seen = result.comments.seen
			w.mu.Unlock()
		}
		panel.status = result.status
		return
	}
	if result.audit != nil {
		panel.audit, panel.auditPage, panel.status = result.audit, 0, result.status
		return
	}
	if result.member != nil {
		panel.applyMember(*result.member)
		panel.status = result.status
		return
	}
	if result.project != nil {
		if result.signature != w.workspaceSignature() {
			panel.status = "Workspace edited while loading. Load the team project again."
			return
		}
		w.finishImport(projectImport{value: result.project})
		if w.collection.project != result.project {
			panel.status = "Save or discard local edits before loading the team project."
			return
		}
	}
	panel.client, panel.workspaceID, panel.revision = result.client, result.id, result.revision
	if panel.client == nil {
		panel.members = nil
		panel.audit = nil
		panel.comments = teamComments{}
		panel.memberEmail.SetText("")
		panel.memberName.SetText("")
	}
	panel.status = result.status
}
