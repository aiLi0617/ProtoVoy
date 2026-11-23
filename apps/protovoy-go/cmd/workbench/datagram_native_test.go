//go:build !js

package main

import "testing"

func TestTCPFramingSurvivesCollectionReload(t *testing.T) {
	for _, framing := range []string{"fixed:8", "delimiter:END:marker"} {
		w := newWorkbench()
		w.datagram = "tcp"
		w.target.SetText("127.0.0.1:9999")
		w.socketFraming.SetText(framing)
		w.socketTLS = true
		w.socketServerName.SetText("{{tlsName}}")
		w.saveDraft()
		if w.collection.project == nil {
			t.Fatal(w.collection.message)
		}
		w.socketFraming.SetText("raw")
		w.loadRequest(1, w.collection.project.Requests[0])
		if w.socketFraming.Text() != framing {
			t.Fatalf("framing changed from %q to %q", framing, w.socketFraming.Text())
		}
		if !w.socketTLS || w.socketServerName.Text() != "{{tlsName}}" {
			t.Fatal("TLS settings were lost")
		}
	}
}

func TestSocketCollectionAndDraftRestore(t *testing.T) {
	t.Setenv("PROTOVOY_CONFIG_DIR", t.TempDir())
	w := newWorkbench()
	w.restoreWorkspace()
	w.datagram = "udp"
	w.target.SetText("127.0.0.1:9999")
	w.method.SetText("hex")
	w.socketTimeout.SetText("1250")
	w.requestBody.input.Editor.Insert("0001ff")
	w.requestName.SetText("Binary UDP")
	w.saveDraft()
	if w.collection.selected != 1 || w.collection.project.Requests[0].Socket == nil {
		t.Fatal("socket request was not saved")
	}
	w.flushWorkspace()
	reopened := newWorkbench()
	reopened.restoreWorkspace()
	if reopened.datagram != "udp" || reopened.method.Text() != "hex" || reopened.requestBody.input.Editor.String() != "0001ff" ||
		reopened.socketTimeout.Text() != "1250" {
		t.Fatal("socket draft did not restore")
	}
	reopened.datagram = ""
	reopened.loadRequest(1, reopened.collection.project.Requests[0])
	if reopened.datagram != "udp" || reopened.target.Text() != "127.0.0.1:9999" {
		t.Fatal("collection load lost socket protocol")
	}
}
