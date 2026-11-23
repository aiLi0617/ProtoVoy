package project

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

const teamSnapshotFixture = `{
  "schemaVersion":1,"exportedAt":"2026-01-01T00:00:00Z",
  "tree":{
    "workspaces":[{"id":"w","name":"Team"}],
    "projects":[{"id":"p","workspaceId":"w","name":"Services"}],
    "modules":[{"id":"m","projectId":"p","name":"API","isDefault":false}],
    "collections":[
      {"id":"child","projectId":"p","parentId":"root","name":"Queue"},
      {"id":"root","projectId":"p","moduleId":"m","parentId":null,"name":"Requests"}
    ],
    "requests":[
      {"id":"h","projectId":"p","collectionId":"root","name":"HTTP"},
      {"id":"q","projectId":"p","collectionId":"child","name":"MQTT"}
    ]
  },
  "requests":[
    {"originalId":"h","projectId":"p","collectionId":"root","envelope":{
      "name":"HTTP","protocolId":"http","target":"http://localhost/start",
      "metadata":{"__protovoyDisabledGeneratedHeaders":[],"__protovoyAssertionsEnabled":true,"__protovoyAssertionSchemaVersion":2},
      "payload":{"type":"http","method":"GET","headers":[["X-Test","{{value}}"]],"followRedirects":true},
      "variables":{"value":"fixture"}
    }},
    {"originalId":"q","projectId":"p","collectionId":"child","envelope":{
      "name":"MQTT","protocolId":"mqtt","target":"mqtt://localhost",
      "payload":{"type":"raw","value":{"topic":"demo","payload":"hello"}}
    }}
  ]
}`

func TestReadTeamSnapshotPreservesPathsAndRequestSettings(t *testing.T) {
	value, err := Read(strings.NewReader(teamSnapshotFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Requests) != 2 {
		t.Fatalf("unexpected request count: %d", len(value.Requests))
	}
	http, mqtt := value.Requests[0], value.Requests[1]
	if !http.FollowRedirects || http.Headers["X-Test"] != "{{value}}" || http.Variables["value"] != "fixture" ||
		strings.Join(http.CollectionPath, "/") != "Team/Services/API/Requests" ||
		mqtt.MQTT == nil || mqtt.MQTT.Topic != "demo" || mqtt.Body != "hello" ||
		strings.Join(mqtt.CollectionPath, "/") != "Team/Services/API/Requests/Queue" {
		t.Fatal("team snapshot lost request or hierarchy settings")
	}
	data, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Read(bytes.NewReader(data))
	if err != nil || !reflect.DeepEqual(restored.Requests, value.Requests) {
		t.Fatalf("snapshot failed v2 round-trip: %v", err)
	}
}

func TestTeamSnapshotPreservesEmptyCollectionFolders(t *testing.T) {
	source := strings.Replace(teamSnapshotFixture, `"name":"Requests"}`, `"name":"Requests"},
      {"id":"empty","projectId":"p","parentId":"root","name":"Drafts"},
      {"id":"nested","projectId":"p","parentId":"empty","name":"Next release"}`, 1)
	value, err := Read(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"Team", "Services", "API", "Requests", "Drafts", "Next release"}}
	if !reflect.DeepEqual(value.EmptyFolders, want) {
		t.Fatalf("empty folder paths = %v, want %v", value.EmptyFolders, want)
	}
	portable, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Read(bytes.NewReader(portable))
	if err != nil || !reflect.DeepEqual(reopened.EmptyFolders, want) {
		t.Fatalf("v2 lost empty folders: %v, %v", reopened.EmptyFolders, err)
	}
	saved, err := EncodeWorkspace(Workspace{Project: &value})
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := DecodeWorkspace(saved)
	if err != nil || workspace.Project == nil || !reflect.DeepEqual(workspace.Project.EmptyFolders, want) {
		t.Fatalf("workspace lost empty folders: %v, %v", workspace.Project, err)
	}
}

func TestReadTeamSnapshotRejectsInvalidHierarchyAndUnsupportedRequests(t *testing.T) {
	for _, change := range [][2]string{
		{`"parentId":null`, `"parentId":"child"`},
		{`"originalId":"q"`, `"originalId":"h"`},
		{`"originalId":"q","projectId":"p","collectionId":"child"`, `"originalId":"q","projectId":"p","collectionId":"root"`},
		{`"topic":"demo"`, `"topic":"demo","passwordRef":" fixture"`},
		{`"protocolId":"mqtt"`, `"protocolId":"unknown"`},
	} {
		value, err := Read(strings.NewReader(strings.Replace(teamSnapshotFixture, change[0], change[1], 1)))
		if err == nil || len(value.Requests) != 0 {
			t.Fatalf("snapshot accepted an invalid migration: %s", change[1])
		}
	}
}

func TestReadTeamSnapshotPreservesKafkaFolder(t *testing.T) {
	source := strings.Replace(teamSnapshotFixture, `"protocolId":"mqtt","target":"mqtt://localhost"`,
		`"protocolId":"kafka","target":"localhost:9092"`, 1)
	value, err := Read(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	request := value.Requests[1]
	if request.Kafka == nil || request.Kafka.Topic != "demo" || request.Body != "hello" ||
		strings.Join(request.CollectionPath, "/") != "Team/Services/API/Requests/Queue" {
		t.Fatal("Kafka request or folder path was lost")
	}
}
