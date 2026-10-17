package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/theapemachine/symm/data"
)

type stubNode struct {
	*System
	hold time.Duration
}

func (stub *stubNode) Step(value *data.Value) *data.Value {
	if stub.hold > 0 {
		time.Sleep(stub.hold)
	}

	return value
}

func TestWorkspaceTraceRecordsWorkAndWait(t *testing.T) {
	ctx := context.Background()
	alpha := &stubNode{System: NewSystem(ctx, "alpha"), hold: time.Millisecond}
	beta := &stubNode{System: NewSystem(ctx, "beta"), hold: time.Millisecond}
	workspace := NewWorkspace(ctx, "trace-test", []Ring{
		{Group: "signal", Nodes: []Node{alpha}},
		{Group: "strategy", Nodes: []Node{beta}},
	})

	if workspace == nil {
		t.Fatal("workspace did not start")
	}

	t.Cleanup(func() { _ = workspace.Close() })
	workspace.Transition(READY)

	var waited sync.WaitGroup
	waited.Add(1)

	var got []data.Span
	workspace.Publish(func(value *data.Value) {
		got = value.Spans()
		waited.Done()
	})

	value := data.NewValue("workspace", "tick", "aggregate")
	value.Note(data.Span{Name: "assemble", Group: "ingress", WorkNs: 2500})
	workspace.Step(value)

	done := make(chan struct{})
	go func() {
		waited.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("trace was not published")
	}

	byName := map[string]data.Span{}

	for _, span := range got {
		byName[span.Name] = span
	}

	for _, name := range []string{"assemble", "alpha", "beta"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing span %s in %+v", name, got)
		}
	}

	if byName["alpha"].Group != "signal" || byName["alpha"].WorkNs < int64(time.Millisecond) {
		t.Fatalf("alpha = %+v", byName["alpha"])
	}

	if byName["beta"].Group != "strategy" || byName["beta"].WorkNs < int64(time.Millisecond) {
		t.Fatalf("beta = %+v", byName["beta"])
	}

	if wire := value.Size(); wire == 0 {
		t.Fatal("value has no wire size")
	}
}

func TestMeasureSplitsWaitWorkAndBacklog(t *testing.T) {
	workspace := &Workspace{}
	workspace.published.Store(8)

	value := data.NewValue("workspace", "tick", "aggregate")
	value.MarkEnqueued(mono() - int64(2*time.Millisecond))

	workspace.measure(value, "alpha", "signal", 0, 5, func() {
		time.Sleep(time.Millisecond)
	})

	spans := value.Spans()

	if len(spans) != 1 {
		t.Fatalf("spans = %+v", spans)
	}

	span := spans[0]

	if span.Name != "alpha" || span.Group != "signal" {
		t.Fatalf("span = %+v", span)
	}

	if span.WaitNs < int64(time.Millisecond) {
		t.Fatalf("wait %dns, want at least 1ms of queue", span.WaitNs)
	}

	if span.WorkNs < int64(time.Millisecond) {
		t.Fatalf("work %dns, want at least 1ms inside the stage", span.WorkNs)
	}

	if span.Backlog != 3 {
		t.Fatalf("backlog = %d, want published 8 minus seq 5", span.Backlog)
	}
}
