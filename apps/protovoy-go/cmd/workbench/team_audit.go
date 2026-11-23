package main

import (
	"context"
	"fmt"
	"time"

	"github.com/vivaui/viva/flex"
)

var (
	teamAuditID     = tree.ID(page, "team-audit")
	teamAuditHideID = tree.ID(page, "team-audit-hide")
	teamAuditPrevID = tree.ID(page, "team-audit-prev")
	teamAuditNextID = tree.ID(page, "team-audit-next")
)

func (w *workbench) drawTeamAudit(f *flex.Frame) {
	panel := &w.team
	client := panel.client
	if client.Role != "OWNER" && client.Role != "ADMIN" {
		return
	}
	if f.Button(teamAuditID, "Load team audit").Clicked() {
		w.runTeamAction("Loading team audit…", func(ctx context.Context) teamActionResult {
			events, err := client.Audit(ctx)
			return teamActionResult{audit: events, err: err, status: "Team audit loaded"}
		})
		return
	}
	if panel.audit == nil {
		return
	}
	if f.Button(teamAuditHideID, "Hide team audit").Clicked() {
		panel.audit = nil
		return
	}
	if len(panel.audit) == 0 {
		f.Text("No audit events")
		return
	}
	const pageSize = 10
	start := panel.auditPage * pageSize
	end := min(start+pageSize, len(panel.audit))
	f.Text(fmt.Sprintf("Audit events %d–%d of %d", start+1, end, len(panel.audit)))
	for _, event := range panel.audit[start:end] {
		actor := event.ActorID
		for _, member := range panel.members {
			if member.ID == actor {
				actor = member.DisplayName
				break
			}
		}
		f.Wrap(event.Action + " · " + event.ResourceType + " " + event.ResourceID)
		f.Wrap(event.CreatedAt.UTC().Format(time.DateTime) + " UTC · " + actor + " · " + string(event.Details)).Muted()
	}
	if panel.auditPage > 0 && f.Button(teamAuditPrevID, "Previous audit events").Clicked() {
		panel.auditPage--
	}
	if end < len(panel.audit) && f.Button(teamAuditNextID, "Next audit events").Clicked() {
		panel.auditPage++
	}
}
