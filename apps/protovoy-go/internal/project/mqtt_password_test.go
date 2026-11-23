package project

import (
	"bytes"
	"strings"
	"testing"
)

func TestMQTTReferencesSurviveProjectAndTeamImport(t *testing.T) {
	for _, input := range []string{
		`{"protocolId":"mqtt","target":"mqtts://broker.example:8883","payload":{"type":"raw","value":{"topic":"jobs","passwordRef":"fixture"}}}`,
		strings.Replace(teamSnapshotFixture, `"topic":"demo"`, `"topic":"demo","passwordRef":"fixture"`, 1),
	} {
		value, err := Read(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, request := range value.Requests {
			if request.MQTT != nil && request.MQTT.PasswordRef == "fixture" {
				found = true
			}
		}
		if !found {
			t.Fatal("import lost MQTT credential reference")
		}
		encoded, err := Encode(value)
		if err != nil {
			t.Fatal(err)
		}
		reopened, err := Read(bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		found = false
		for _, request := range reopened.Requests {
			if request.MQTT != nil && request.MQTT.PasswordRef == "fixture" {
				found = true
			}
		}
		if !found {
			t.Fatal("portable project lost MQTT credential reference")
		}
	}
}
