package runtime

import (
	"context"
	"errors"
	"sync/atomic"

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
	status  *Status
	channel disruptor.Disruptor
	buffer  []T
	// headSeq is the highest sequence Push has committed to the ring — the
	// producer's position. A BacklogStepper compares it against its own
	// envelope's sequence (see Consumer.Handle) to read real ring pressure
	// straight off the same numbers the disruptor uses for backpressure,
	// never estimated from rates.
	headSeq atomic.Int64
}

func NewWorkload[T any](
	ctx context.Context,
	stages [][]Node[T],
) *Workload[T] {
	ctx, cancel := context.WithCancel(ctx)

	workload := &Workload[T]{
		ctx:    ctx,
		cancel: cancel,
		status: NewStatus(),
		buffer: make([]T, system.Cfg.Runtime.Workspace.Buffer),
	}
	workload.headSeq.Store(-1)

	opts := optionList(
		disruptor.Options.BufferCapacity(
			system.Cfg.Runtime.Workspace.Buffer,
		),
	)

	for _, stage := range stages {
		group := make([]disruptor.Handler, len(stage))

		for i, node := range stage {
			group[i] = NewConsumer(node, workload.buffer, &workload.headSeq)
		}

		if len(group) > 0 {
			opts = append(opts, disruptor.Options.NewHandlerGroup(group...))
		}
	}

	workload.channel, workload.err = disruptor.New(
		opts...,
	)

	go workload.channel.Listen()
	workload.status.Transition(READY)
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
	workload.headSeq.Store(seq)
}

func (workload *Workload[T]) Close() error {
	workload.cancel()
	return workload.channel.Close()
}

/*
Status reports the workload's readiness. It is READY once the ring and its
listener are live — i.e. the moment NewWorkload returns — so a consumer can
gate on it before producing into the ring.
*/
func (workload *Workload[T]) Status() *Status {
	return workload.status
}
