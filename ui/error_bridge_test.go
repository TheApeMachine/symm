package ui

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
TestErrorBridgeForwardsErrorLevel publishes error frames and ignores info.
*/
func TestErrorBridgeForwardsErrorLevel(t *testing.T) {
	t.Parallel()

	ui := transport.NewMapReduce[[]byte]([]string{"test"}, nil, nil)
	bridge := &ErrorBridge{ui: ui, ready: func() bool { return true }}

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

	// Only the error line is published.
	if ui.Length() != 1 {
		t.Fatalf("want 1 error frame, got %d", ui.Length())
	}

	frame, _ := ui.Pop("test")
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
}

/*
TestErrorBridgeDedupesIdenticalFlood drops repeats inside the debounce window.
*/
func TestErrorBridgeDedupesIdenticalFlood(t *testing.T) {
	t.Parallel()

	ui := transport.NewMapReduce[[]byte]([]string{"test"}, nil, nil)
	bridge := &ErrorBridge{ui: ui, ready: func() bool { return true }}
	line := []byte(`{"level":"error","error":"same"}`)

	_, _ = bridge.Write(line)
	_, _ = bridge.Write(line)

	if got := ui.Length(); got != 1 {
		t.Fatalf("want 1 frame after debounce, got %d", got)
	}
}

/*
TestErrorBridgeWithholdsUntilReady drops error frames before the Warmup gate.
*/
func TestErrorBridgeWithholdsUntilReady(t *testing.T) {
	t.Parallel()

	ui := transport.NewMapReduce[[]byte]([]string{"test"}, nil, nil)
	open := false
	bridge := &ErrorBridge{
		ui:    ui,
		ready: func() bool { return open },
	}
	line := []byte(`{"level":"error","error":"allocator: ask unavailable for BILL/USD"}`)

	_, _ = bridge.Write(line)

	if got := ui.Length(); got != 0 {
		t.Fatalf("want 0 frames before Warmup, got %d", got)
	}

	open = true
	_, _ = bridge.Write(line)

	if got := ui.Length(); got != 1 {
		t.Fatalf("want 1 frame after Warmup, got %d", got)
	}
}

/*
TestErrorBridgeDoesNotBlock returns immediately even with no consumer draining,
because the underlying transport is an unbounded lock-free queue.
*/
func TestErrorBridgeDoesNotBlock(t *testing.T) {
	t.Parallel()

	ui := transport.NewMapReduce[[]byte](nil, nil, nil)
	bridge := &ErrorBridge{ui: ui, ready: func() bool { return true }}
	done := make(chan struct{})

	go func() {
		_, _ = bridge.Write([]byte(`{"level":"error","error":"full"}`))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Write blocked")
	}
}
