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

func TestWorkspacePublishesTheTick(t *testing.T) {
	ctx := context.Background()
	alpha := &stubNode{System: NewSystem(ctx, "alpha")}
	beta := &stubNode{System: NewSystem(ctx, "beta")}
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

	var got *data.Value
	workspace.Publish(func(value *data.Value) {
		got = value
		waited.Done()
	})

	value := data.NewValue("workspace", "tick", "aggregate")
	workspace.Step(value)

	done := make(chan struct{})
	go func() {
		waited.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("tick was not published")
	}

	if got != value {
		t.Fatalf("published %p, stepped %p", got, value)
	}
}
