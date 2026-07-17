package ui

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
)

/*
TestErrorBridgeForwardsErrorLevel publishes error frames and ignores info.
*/
func TestErrorBridgeForwardsErrorLevel(t *testing.T) {
	t.Parallel()

	messages := make(chan []byte, 2)
	bridge := &ErrorBridge{messages: messages}

	info := []byte(`{"level":"info","message":"noise"}`)
	errorLine := []byte(
		`{"level":"error","error":"logic causal: manifold chronology regressed","caller":"logic/causal.go:87"}`,
	)

	if _, err := bridge.Write(info); err != nil {
		t.Fatal(err)
	}

	if _, err := bridge.Write(errorLine); err != nil {
		t.Fatal(err)
	}

	select {
	case frame := <-messages:
		var parsed map[string]any

		if err := sonic.Unmarshal(frame, &parsed); err != nil {
			t.Fatal(err)
		}

		payload, ok := parsed["error"].(map[string]any)

		if !ok {
			t.Fatalf("want error object, got %#v", parsed)
		}

		if payload["error"] != "logic causal: manifold chronology regressed" {
			t.Fatalf("unexpected payload %#v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("expected error frame")
	}

	select {
	case frame := <-messages:
		t.Fatalf("info must not publish, got %s", frame)
	default:
	}
}

/*
TestErrorBridgeDedupesIdenticalFlood drops repeats inside the debounce window.
*/
func TestErrorBridgeDedupesIdenticalFlood(t *testing.T) {
	t.Parallel()

	messages := make(chan []byte, 4)
	bridge := &ErrorBridge{messages: messages}
	line := []byte(`{"level":"error","error":"same"}`)

	_, _ = bridge.Write(line)
	_, _ = bridge.Write(line)

	if got := len(messages); got != 1 {
		t.Fatalf("want 1 frame after debounce, got %d", got)
	}
}

/*
TestErrorBridgeDoesNotBlockWhenChannelFull returns immediately on a full hub.
*/
func TestErrorBridgeDoesNotBlockWhenChannelFull(t *testing.T) {
	t.Parallel()

	messages := make(chan []byte)
	bridge := &ErrorBridge{messages: messages}
	done := make(chan struct{})

	go func() {
		_, _ = bridge.Write([]byte(`{"level":"error","error":"full"}`))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Write blocked on full channel")
	}
}
