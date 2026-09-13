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

type Workload[T any] struct {
	*System
	channel  disruptor.Disruptor
	buffer   []T
	register *store.Register[T]
	stages   [][]Node[T]
}

func NewWorkload[T any](
	ctx context.Context,
	label string,
	stages [][]Node[T],
	registers ...*store.Register[T],
) *Workload[T] {
	reg := store.NewRegister[T]()

	if len(registers) > 0 && registers[0] != nil {
		reg = registers[0]
	}

	workload := &Workload[T]{
		buffer:   make([]T, system.Cfg.Runtime.Workspace.Buffer),
		register: reg,
		stages:   stages,
	}

	workload.System = NewSystem(ctx, label)

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


func (workload *Workload[T]) Step(payload T) T {
	select {
	case <-workload.Context().Done():
		workload.Error(workload.Context().Err())
		return payload
	default:
		if workload.Error() != nil || workload.Status() != READY {
			return payload
		}
	}

	// Reserve 1 slot on the ring buffer
	seq := workload.channel.Reserve(1)

	// Write payload to ring buffer slot
	slot := &workload.buffer[seq&system.Cfg.Runtime.Workspace.Mask]
	*slot = payload

	// Make available to Stage 1
	workload.channel.Commit(seq, seq)
	return payload
}

func (workload *Workload[T]) Register() T {
	var zero T
	return zero
}

func (workload *Workload[T]) Transition(stage Stage) {
	workload.System.Transition(stage)

	for _, stageGroup := range workload.stages {
		for _, node := range stageGroup {
			if sys, ok := any(node).(interface{ Transition(Stage) }); ok {
				sys.Transition(stage)
			}
		}
	}
}
