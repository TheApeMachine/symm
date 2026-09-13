package runtime

import (
	"context"
	"errors"

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
}

func NewWorkload[T any](
	ctx context.Context,
	label string,
	stages [][]Node[T],
) *Workload[T] {
	workload := &Workload[T]{
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

func (workload *Workload[T]) Step(payload T) T {
	select {
	case <-workload.ctx.Done():
		workload.err = errors.Join(
			workload.err, workload.ctx.Err(),
		)

		return payload
	default:
		if workload.err != nil {
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

/*
Close shuts the ring down and then closes the embedded system's lifecycle.
*/
func (workload *Workload[T]) Close() error {
	if err := workload.channel.Close(); err != nil {
		workload.err = errors.Join(workload.err, err)
	}

	return workload.System.Close()
}
