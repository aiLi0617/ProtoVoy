package project

import (
	"testing"

	"protovoy.dev/protovoy/internal/execution"
)

func TestWorkspaceRedisRoundTrip(t *testing.T) {
	settings := RedisSettings{Protocol: 3, Username: "tester", Database: "2", TLS: true, ServerName: "redis.example"}
	draft := Draft{Redis: settings, Name: "Redis test", Target: "localhost:6379", Body: `[["SET","key","{{value}}"]]`}
	request := Request{Redis: &settings, Name: draft.Name, URL: draft.Target, Body: draft.Body}
	data, err := EncodeWorkspace(Workspace{Project: &Project{Requests: []Request{request}}, Draft: draft, Selected: 1})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := DecodeWorkspace(data)
	if err != nil || saved.Draft != draft || saved.Selected != 1 || len(saved.Project.Requests) != 1 {
		t.Fatalf("could not restore Redis workspace: %+v, %v", saved, err)
	}
	restored := saved.Project.Requests[0]
	if restored.Redis == nil || *restored.Redis != settings || restored.Body != request.Body {
		t.Fatalf("could not restore Redis request: %+v", restored)
	}
}

func TestWorkspaceRefusesRedisCredentialCommands(t *testing.T) {
	settings := RedisSettings{Protocol: 3}
	for _, body := range []string{`[["aUtH","fixture"]]`, `[["HELLO","3","AUTH","user","fixture"]]`,
		`[["ACL","SETUSER","user",">fixture"]]`, `[["MIGRATE","host","6379","key","0","5000","AUTH2","user","fixture"]]`,
		`[["CONFIG","SET","masterauth","fixture"]]`, `[["{{command}}","fixture"]]`, `[["AUTH","fixture"`} {
		value := Workspace{Draft: Draft{Redis: settings, Body: body}}
		if data, err := EncodeWorkspace(value); err == nil || data != nil {
			t.Fatal("unsafe Redis draft was serialized")
		}
		value = Workspace{Project: &Project{Requests: []Request{{Redis: &settings, Body: body}}}}
		if data, err := EncodeWorkspace(value); err == nil || data != nil {
			t.Fatal("unsafe Redis request was serialized")
		}
	}
}

func TestWorkspaceRefusesPrivateKeyInCAField(t *testing.T) {
	for _, saved := range []bool{false, true} {
		value := Workspace{}
		privateKey := "-----BEGIN PRIVATE KEY-----\nexample\n-----END PRIVATE KEY-----"
		if saved {
			value.Project = &Project{Requests: []Request{{Socket: &execution.SocketPayload{CAPEM: privateKey}}}}
		} else {
			value.Draft.SocketCA = privateKey
		}
		if data, err := EncodeWorkspace(value); err == nil || data != nil {
			t.Fatal("private key was serialized")
		}
	}
}
