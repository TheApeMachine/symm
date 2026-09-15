package runtime

import (
	"context"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
)

func optionList[O any](initial ...O) []O {
	return initial
}

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
	channel   disruptor.Disruptor
	buffer    []T
	register  *store.Register[T]
	consumers []*Consumer[T]
	tees      []Tee
}

func NewWorkspace[T any](
	ctx context.Context,
	label string,
	stages [][]Node[T],
	tees ...Tee,
) *Workspace[T] {
	workload := &Workspace[T]{
		buffer:   make([]T, system.Cfg.Runtime.Workspace.Buffer),
		register: store.NewRegister[T](),
		tees:     tees,
	}

	opts := optionList(
		disruptor.Options.BufferCapacity(
			system.Cfg.Runtime.Workspace.Buffer,
		),
	)

	for _, stage := range stages {
		group := make([]disruptor.Handler, len(stage))

		for index, node := range stage {
			consumer := NewConsumer(ctx, node, workload.register, tees...)
			workload.consumers = append(workload.consumers, consumer)
			group[index] = consumer
		}

		if len(group) > 0 {
			opts = append(opts, disruptor.Options.NewHandlerGroup(group...))
		}
	}

	channel, err := disruptor.New(opts...)
	workload.System = NewSystem(ctx, label, channel, workload)

	if err != nil {
		workload.Error(errnie.Err(
			errnie.Internal,
			"[workspace] error creating LMAX disruptor channel",
			err,
		))

		return nil
	}

	workload.channel = channel

	go workload.channel.Listen()
	return workload
}

// Transition opens downstream consumers before workspace admission. Pausing the
// workspace stops new admission while already committed work can finish.
func (workspace *Workspace[T]) Transition(stage Stage) {
	if stage == READY {
		for index := len(workspace.consumers) - 1; index >= 0; index-- {
			workspace.consumers[index].Transition(READY)
		}
	}

	workspace.System.Transition(stage)
}

// Step drops input without reserving a ring slot until the workspace is ready.
func (workspace *Workspace[T]) Step(payload T) T {
	if workspace.Status() != READY {
		errnie.Warn(workspace.Name() + ": Step called before READY; dropping event")
		return payload
	}

	select {
	case <-workspace.Context().Done():
		workspace.Error(workspace.Context().Err())
		return payload
	default:
		if workspace.Error() != nil {
			workspace.Error(errnie.Err(
				errnie.Internal,
				"[workspace] internal error encountered",
				nil,
			))
			return payload
		}
	}

	seq := workspace.channel.Reserve(1)
	slot := &workspace.buffer[seq&system.Cfg.Runtime.Workspace.Mask]
	*slot = payload
	workspace.channel.Commit(seq, seq)

	return payload
}
