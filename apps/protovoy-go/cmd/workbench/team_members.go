package main

import (
	"context"
	"strings"

	"github.com/vivaui/viva/flex"
	"protovoy.dev/protovoy/internal/teamclient"
)

var (
	teamMembersID      = tree.ID(page, "team-members")
	teamMemberEmailID  = tree.ID(page, "team-member-email")
	teamMemberNameID   = tree.ID(page, "team-member-name")
	teamMemberPassID   = tree.ID(page, "team-member-password")
	teamMemberRoleID   = tree.Sub(page, tree.Name("team-member-role"))
	teamMemberCreateID = tree.ID(page, "team-member-create")
	teamMemberUpdateID = tree.ID(page, "team-member-update")
)

func (w *workbench) drawTeamMembers(f *flex.Frame) {
	panel := &w.team
	client := panel.client
	if f.Button(teamMembersID, "Load team members").Clicked() {
		w.runTeamAction("Loading team members…", func(ctx context.Context) teamActionResult {
			members, err := client.Members(ctx)
			return teamActionResult{members: members, status: "Team members loaded", err: err}
		})
		return
	}
	for _, member := range panel.members {
		f.Wrap(member.DisplayName + " (" + member.Email + ") · " + member.Role)
	}
	if client.Role != "OWNER" && client.Role != "ADMIN" {
		return
	}
	roles := []string{"VIEWER", "RUNNER", "EDITOR", "ADMIN"}
	if client.Role == "OWNER" {
		roles = append(roles, "OWNER")
	}
	panel.memberRoleIndex = min(panel.memberRoleIndex, len(roles)-1)
	panel.memberPassword.Secure = true
	f.Field(&panel.memberEmail, teamMemberEmailID, "Member email", "Member email")
	f.Field(&panel.memberName, teamMemberNameID, "New member display name", "Display name")
	f.Field(&panel.memberPassword, teamMemberPassID, "New member password", "New password")
	if selected, ok := f.Select(teamMemberRoleID, &panel.memberRole, roles, panel.memberRoleIndex).Placeholder("Member role").Picked(); ok {
		panel.memberRoleIndex = selected
	}
	role := roles[panel.memberRoleIndex]
	if f.Button(teamMemberCreateID, "Create member account").Clicked() {
		w.saveTeamMember(role, true)
	} else if f.Button(teamMemberUpdateID, "Assign role").Clicked() {
		w.saveTeamMember(role, false)
	}
}

func (w *workbench) saveTeamMember(role string, create bool) {
	panel := &w.team
	client := panel.client
	email, name := strings.TrimSpace(panel.memberEmail.Text()), strings.TrimSpace(panel.memberName.Text())
	password := panel.memberPassword.Text()
	panel.memberPassword.SetText("")
	w.runTeamAction("Saving team member…", func(ctx context.Context) teamActionResult {
		member, err := client.SaveMember(ctx, email, name, password, role, create)
		return teamActionResult{member: &member, status: "Saved team member", err: err}
	})
}

func (panel *teamPanel) applyMember(member teamclient.Member) {
	if member.ID == panel.client.UserID {
		panel.client.Role = member.Role
		if member.Role != "OWNER" && member.Role != "ADMIN" {
			panel.audit = nil
		}
	}
	for index := range panel.members {
		if panel.members[index].ID == member.ID {
			panel.members[index] = member
			return
		}
	}
	panel.members = append(panel.members, member)
}
