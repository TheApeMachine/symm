package runtime

import (
	"context"
)

/*
Workspace is SYMM's real-time streaming execution fabric. Every node declares
exactly two things at registration: the type it wants and the type it returns.
The workspace is the sole router — when it has a value of some type, it calls
Step on every node that wants that type, on that node's own dedicated ring, and
recursively dispatches whatever each Step returns the same way. There is no
topic string, and nothing ever calls a "Publish" method to hand a value to the
bus: a node's Step return value IS its emission, and Feed is the only entry
point for a value with no upstream producer (e.g. a value parsed off a
websocket).
*/
type Workspace[T any] struct {
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	workloads []*Workload[T]
}

func NewWorkspace[T any](
	ctx context.Context, workloads []*Workload[T],
) *Workspace[T] {
	ctx, cancel := context.WithCancel(ctx)

	workspace := &Workspace[T]{
		ctx:       ctx,
		cancel:    cancel,
		workloads: workloads,
	}

	return workspace
}

/* Admit opens every workload as one generation after subscription completes. */
func (workspace *Workspace[T]) Admit() {
	if workspace == nil {
		return
	}

	for _, workload := range workspace.workloads {
		workload.Admit()
	}
}

func (workspace *Workspace[T]) Close() error {
	workspace.cancel()
	return nil
}
