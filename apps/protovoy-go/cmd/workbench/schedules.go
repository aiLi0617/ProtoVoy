package main

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/vivaui/viva/flex"
	"protovoy.dev/protovoy/internal/scheduleclient"
	"protovoy.dev/protovoy/internal/scheduler"
)

var schedulesID = tree.ID(page, "schedules")

type schedulePanel struct {
	open                                   bool
	origin, user, password, name, interval flex.Field
	client                                 *scheduleclient.Client
	jobs                                   []scheduler.Job
	results                                []scheduler.Execution
	status                                 string
	pending                                *scheduleActionResult // protected by workbench.mu
}

type scheduleActionResult struct {
	client  *scheduleclient.Client
	jobs    []scheduler.Job
	results []scheduler.Execution
	err     error
}

func (w *workbench) drawSchedules(f *flex.Frame) {
	p := &w.schedules
	f.Text("Scheduled requests")
	if w.cancel != nil {
		if f.Button(cancelID, "Cancel schedule operation").Clicked() {
			w.cancel()
		}
		f.Wrap(p.status)
		return
	}
	if p.client == nil {
		if teamServerOrigin() == "" {
			f.Field(&p.origin, tree.ID(page, "schedule-origin"), "Schedule server origin", "https://workbench.example.com")
			f.Field(&p.user, tree.ID(page, "schedule-user"), "Hosted username", "Username")
			p.password.Secure = true
			f.Field(&p.password, tree.ID(page, "schedule-password"), "Hosted password", "Password")
		}
		if f.Button(tree.ID(page, "schedule-connect"), "Connect to schedules").Clicked() {
			w.connectSchedules()
		}
		f.Wrap("Enable scheduling on the server with a schedule database. Jobs run on that server using its configured targets.").Muted()
	} else {
		if f.Button(tree.ID(page, "schedule-refresh"), "Refresh schedules").Clicked() {
			w.refreshSchedules(nil)
			return
		}
		if f.Button(tree.ID(page, "schedule-disconnect"), "Disconnect schedules").Clicked() {
			p.client = nil
			p.jobs = nil
			p.results = nil
			p.password.SetText("")
			p.status = "Disconnected"
			return
		}
		if w.collection.project != nil && w.collection.selected > 0 {
			request := w.collection.project.Requests[w.collection.selected-1]
			f.Wrap("Saved request: " + request.Name)
			f.Field(&p.name, tree.ID(page, "schedule-name"), "Schedule name", "Schedule name")
			f.Field(&p.interval, tree.ID(page, "schedule-interval"), "Interval in seconds", "60")
			f.Wrap("Creates a disabled job from the saved request and saved collection variables. Review the job before enabling it.").Muted()
			if f.Button(tree.ID(page, "schedule-create"), "Create disabled schedule").Clicked() {
				w.createSchedule()
				return
			}
		} else {
			f.Wrap("Save and select a request in the workbench to create a schedule.").Muted()
		}
		f.Text(fmt.Sprintf("Jobs (%d)", len(p.jobs)))
		for _, job := range p.jobs {
			state := "Disabled"
			label := "Enable"
			if job.Enabled {
				state = "Enabled"
				label = "Disable"
			}
			f.Wrap(fmt.Sprintf("%s · %s · every %ds", job.Name, state, job.IntervalSeconds))
			f.Wrap("Next run: " + job.NextRunAt.Local().Format(time.RFC3339)).Muted()
			if f.Button(tree.ID(page, "schedule-toggle-"+job.ID), label+" "+job.Name).Clicked() {
				w.refreshSchedules(func(ctx context.Context, c *scheduleclient.Client) error {
					_, err := c.SetEnabled(ctx, job.ID, !job.Enabled)
					return err
				})
				return
			}
			if f.Button(tree.ID(page, "schedule-delete-"+job.ID), "Delete "+job.Name).Clicked() {
				w.refreshSchedules(func(ctx context.Context, c *scheduleclient.Client) error { return c.Delete(ctx, job.ID) })
				return
			}
		}
		f.Text("Recent executions")
		for i, result := range p.results {
			if i >= 20 {
				f.Wrap("Showing the latest 20 results.").Muted()
				break
			}
			state := "Failed"
			if result.Success {
				state = "Passed"
			}
			name := result.JobID
			for _, job := range p.jobs {
				if job.ID == result.JobID {
					name = job.Name
					break
				}
			}
			f.Wrap(fmt.Sprintf("%s · %s · %s · %dms", name, state, result.StartedAt.Local().Format(time.RFC3339), result.DurationMS))
		}
	}
	f.Wrap(p.status)
}

func (w *workbench) connectSchedules() {
	p := &w.schedules
	origin := teamServerOrigin()
	if origin == "" {
		origin = strings.TrimSpace(p.origin.Text())
	}
	client, err := scheduleclient.New(origin, nil)
	if err != nil {
		p.status = err.Error()
		return
	}
	if teamServerOrigin() == "" {
		client.SetCredentials(p.user.Text(), p.password.Text())
	}
	p.password.SetText("")
	w.runScheduleAction(client, nil)
}

func (w *workbench) refreshSchedules(action func(context.Context, *scheduleclient.Client) error) {
	w.runScheduleAction(w.schedules.client, action)
}

func (w *workbench) createSchedule() {
	p := &w.schedules
	interval := 60
	var err error
	if strings.TrimSpace(p.interval.Text()) != "" {
		interval, err = strconv.Atoi(strings.TrimSpace(p.interval.Text()))
	}
	if err != nil || interval < 10 || interval > 31536000 {
		p.status = "Enter an interval from 10 to 31536000 seconds"
		return
	}
	request := w.collection.project.Requests[w.collection.selected-1]
	variables := maps.Clone(w.collection.project.CollectionVariables)
	if variables == nil {
		variables = map[string]string{}
	}
	maps.Copy(variables, request.Variables)
	request.Variables = variables
	name := strings.TrimSpace(p.name.Text())
	if name == "" {
		name = request.Name
	}
	w.refreshSchedules(func(ctx context.Context, c *scheduleclient.Client) error {
		_, err := c.Create(ctx, name, interval, false, request)
		return err
	})
}

func (w *workbench) runScheduleAction(client *scheduleclient.Client, action func(context.Context, *scheduleclient.Client) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	w.cancel, w.schedules.status = cancel, "Loading schedules…"
	go func() {
		defer cancel()
		result := scheduleActionResult{client: client}
		if action != nil {
			result.err = action(ctx, client)
		}
		if result.err == nil {
			result.jobs, result.err = client.Jobs(ctx)
		}
		if result.err == nil {
			result.results, result.err = client.Executions(ctx)
		}
		w.mu.Lock()
		w.schedules.pending = &result
		w.mu.Unlock()
		w.invalidator.Invalidate()
	}()
}

func (w *workbench) finishScheduleAction() {
	w.mu.Lock()
	result := w.schedules.pending
	w.schedules.pending = nil
	w.mu.Unlock()
	if result == nil {
		return
	}
	w.cancel = nil
	if result.err != nil {
		w.schedules.status = result.err.Error()
		return
	}
	w.schedules.client, w.schedules.jobs, w.schedules.results = result.client, result.jobs, result.results
	w.schedules.status = "Schedules updated"
}
