//go:build !js

package scripting

import (
	"context"
	"errors"
	"testing"
)

func TestRunRequestScriptInSandbox(t *testing.T) {
	result, err := Run(t.Context(), []string{`
		assert(typeof std === 'undefined' && typeof os === 'undefined' && typeof fetch === 'undefined');
		assert(response.status === 200);
		assert(crypto.sha256('abc') === 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad');
		assert(crypto.base64Encode('你好') === '5L2g5aW9');
		assert(crypto.base64Decode('5L2g5aW9') === '你好');
		const uuid = crypto.randomUUID();
		assert(/^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$/.test(uuid));
		assert(uuid !== crypto.randomUUID());
		request.target += '/jobs';
		variables.id = '42';
		console.log('ready', variables.id);
	`}, []byte(`{"target":"https://example.test"}`), nil, []byte(`{"status":200}`))
	if err != nil || string(result.Request) != `{"target":"https://example.test/jobs"}` ||
		result.Variables["id"] != "42" || len(result.Logs) != 1 || result.Logs[0] != "ready 42" {
		t.Fatalf("unexpected script result: %v, %v", result, err)
	}
}

func TestRunInterruptsInfiniteScript(t *testing.T) {
	_, err := Run(t.Context(), []string{"while (true) {}"}, []byte("{}"), nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected script timeout, got %v", err)
	}
}

func TestSandboxRejectsMemoryExhaustionAndRecoversForNextRequest(t *testing.T) {
	_, err := Run(t.Context(), []string{"const huge = new Uint8Array(128 * 1024 * 1024);"}, []byte("{}"), nil, nil)
	if err == nil {
		t.Fatal("sandbox allowed allocation beyond guest memory limit")
	}
	if _, err := Run(t.Context(), []string{"assert(typeof std === 'undefined' && typeof os === 'undefined');"},
		[]byte("{}"), nil, nil); err != nil {
		t.Fatalf("new sandbox failed after memory exhaustion: %v", err)
	}
}
