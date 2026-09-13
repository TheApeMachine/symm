package runtime

import (
	"context"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
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
	*System
	channel  disruptor.Disruptor
	buffer   []T
	register *store.Register[T]
}

func NewWorkspace[T any](
	ctx context.Context, label string, stages [][]Node[T],
) *Workspace[T] {
	workload := &Workspace[T]{
		System:   NewSystem(ctx, label),
		buffer:   make([]T, system.Cfg.Runtime.Workspace.Buffer),
		register: store.NewRegister[T](),
	}

	opts := optionList(
		disruptor.Options.BufferCapacity(
			system.Cfg.Runtime.Workspace.Buffer,
		),
	)

	for _, stage := range stages {
		group := make([]disruptor.Handler, len(stage))

		for i, node := range stage {
			group[i] = NewConsumer(node, workload.register)
		}

		if len(group) > 0 {
			opts = append(opts, disruptor.Options.NewHandlerGroup(group...))
		}
	}

	workload.channel, workload.err = disruptor.New(
		opts...,
	)

	go workload.channel.Listen()
	return workload
}

func (workspace *Workspace[T]) Close() error {
	workspace.cancel()
	return nil
}
