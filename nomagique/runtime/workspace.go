package runtime

import (
	"context"

	"github.com/smarty/go-disruptor"
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
	channel  disruptor.Disruptor
	buffer   []T
	register *store.Register[T]
	stages   [][]Node[T]
}

func NewWorkspace[T any](
	ctx context.Context, label string, stages [][]Node[T],
	registers ...*store.Register[T],
) *Workspace[T] {
	reg := store.NewRegister[T]()

	if len(registers) > 0 && registers[0] != nil {
		reg = registers[0]
	}

	workload := &Workspace[T]{
		buffer:   make([]T, system.Cfg.Runtime.Workspace.Buffer),
		register: reg,
		stages:   stages,
	}

	workload.System = NewSystem(ctx, label, workload)

	opts := optionList(
		disruptor.Options.BufferCapacity(
			system.Cfg.Runtime.Workspace.Buffer,
		),
	)

	slotOffset := 0

	for _, stage := range stages {
		group := make([]disruptor.Handler, len(stage))

		for index, node := range stage {
			consumer := NewConsumer(node, workload.register)
			consumer.SetPeerLimit(slotOffset)
			group[index] = consumer
		}

		slotOffset += len(stage)

		if len(group) > 0 {
			opts = append(opts, disruptor.Options.NewHandlerGroup(group...))
		}
	}

	channel, err := disruptor.New(opts...)

	if err != nil {
		workload.Error(err)
		return workload
	}

	workload.channel = channel
	workload.AddCloser(channel)
	workload.Transition(READY)
	go workload.channel.Listen()
	return workload
}

func (workspace *Workspace[T]) Step(payload T) T {
	select {
	case <-workspace.Context().Done():
		workspace.Error(workspace.Context().Err())
		return payload
	default:
		if workspace.Error() != nil || workspace.Status() != READY {
			return payload
		}
	}

	seq := workspace.channel.Reserve(1)
	slot := &workspace.buffer[seq&system.Cfg.Runtime.Workspace.Mask]
	*slot = payload
	workspace.channel.Commit(seq, seq)

	return payload
}

func (workspace *Workspace[T]) Transition(stage Stage) {
	workspace.System.Transition(stage)

	for _, stageGroup := range workspace.stages {
		for _, node := range stageGroup {
			if sys, ok := any(node).(interface{ Transition(Stage) }); ok {
				sys.Transition(stage)
			}
		}
	}
}
