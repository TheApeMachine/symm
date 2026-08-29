package runtime

import (
	"context"
	"errors"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/symm/system"
)

func optionList[O any](initial ...O) []O {
	return initial
}

type Workload[T any] struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	channel disruptor.Disruptor
	buffer  []T
}

func NewWorkload[T any](
	ctx context.Context,
	stages [][]Node[T],
) *Workload[T] {
	ctx, cancel := context.WithCancel(ctx)

	workload := &Workload[T]{
		ctx:    ctx,
		cancel: cancel,
		buffer: make([]T, system.Cfg.Runtime.Workspace.Buffer),
	}

	opts := optionList(
		disruptor.Options.BufferCapacity(
			system.Cfg.Runtime.Workspace.Buffer,
		),
	)

	for _, stage := range stages {
		group := make([]disruptor.Handler, len(stage))

		for i, node := range stage {
			group[i] = NewConsumer(node, workload.buffer)
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

func (workload *Workload[T]) Push(payload T) {
	select {
	case <-workload.ctx.Done():
		workload.err = errors.Join(
			workload.err, workload.ctx.Err(),
		)

		return
	default:
		if workload.err != nil {
			return
		}
	}

	// Reserve 1 slot on the ring buffer
	seq := workload.channel.Reserve(1)

	// Write payload to ring buffer slot
	slot := &workload.buffer[seq&system.Cfg.Runtime.Workspace.Mask]
	*slot = payload

	// Make available to Stage 1
	workload.channel.Commit(seq, seq)
}

func (workload *Workload[T]) Close() error {
	workload.cancel()
	return workload.channel.Close()
}
