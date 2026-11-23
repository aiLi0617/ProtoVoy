package main

import (
	"bytes"
	"testing"

	"protovoy.dev/protovoy/internal/project"
)

func TestRedisSaveKeepsPasswordOutOfWorkspace(t *testing.T) {
	w := newWorkbench()
	w.loadRequest(0, project.Request{Redis: &project.RedisSettings{Protocol: 3}})
	w.requestName.SetText("Redis sample")
	w.redis.target.SetText("localhost:6379")
	w.redis.username.SetText("tester")
	w.redis.password.SetText("runtime-only-fixture")
	w.redis.commands.input.Editor.Insert(`[["PING"]]`)
	w.saveDraft()
	if w.collection.selected != 1 || w.redis.password.Text() != "runtime-only-fixture" {
		t.Fatal("Redis save did not keep the selected request and runtime password")
	}
	data, err := project.EncodeWorkspace(project.Workspace{
		Project: w.collection.project, Draft: w.draft(), Selected: w.collection.selected,
	})
	if err != nil || bytes.Contains(data, []byte("runtime-only-fixture")) {
		t.Fatalf("Redis workspace save failed or serialized the password: %v", err)
	}
	w.loadRequest(1, w.collection.project.Requests[0])
	if !w.redis.open || !w.redis.resp3 || w.redis.username.Text() != "tester" ||
		w.redis.commands.input.Editor.String() != `[["PING"]]` || w.redis.password.Text() != "" {
		t.Fatal("loading Redis request did not restore its reusable settings and clear the runtime password")
	}
}
