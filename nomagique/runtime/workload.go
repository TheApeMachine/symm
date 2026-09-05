package runtime

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/symm/system"
)

/*
Workload owns one disruptor whose handler groups are declared as Node stages.

Nodes in one stage run concurrently. The disruptor barrier completes the whole
stage before the next stage advances. Workload also implements Node, allowing
the same staged composition to be nested in a Workspace.
*/
type Workload[T any] struct {
	ctx       context.Context
	cancel    context.CancelFunc
	name      string
	err       error
	status    *Status
	channel   disruptor.Disruptor
	buffer    []T
	headSeq   atomic.Int64
	completed atomic.Int64
	output    Node[T]
}

/*
NewWorkload constructs one independently running processing ring. The name
identifies this ring to any Composed node it owns, which is how a stage can
report the ring it runs in rather than leaving a consumer to infer grouping
from naming conventions.
*/
func NewWorkload[T any](
	ctx context.Context,
	name string,
	stages [][]Node[T],
) *Workload[T] {
	return newWorkload(ctx, name, stages, 1)
}

func newWorkload[T any](
	ctx context.Context,
	name string,
	stages [][]Node[T],
	writers uint8,
) *Workload[T] {
	ctx, cancel := context.WithCancel(ctx)
	workload := &Workload[T]{
		ctx:    ctx,
		cancel: cancel,
		name:   name,
		status: NewStatus(),
		buffer: make([]T, system.Cfg.Runtime.Workspace.Buffer),
	}
	workload.headSeq.Store(-1)
	workload.completed.Store(-1)
	workload.status.Transition(WAITING)
	workload.start(stages, writers)

	return workload
}

func (workload *Workload[T]) start(stages [][]Node[T], writers uint8) {
	if writers < 1 {
		workload.err = errors.New("runtime: workload requires at least one writer")

		return
	}

	options := optionList(
		disruptor.Options.BufferCapacity(system.Cfg.Runtime.Workspace.Buffer),
		disruptor.Options.WriterCount(writers),
	)

	for index, stage := range stages {
		group := make([]disruptor.Handler, 0, len(stage))

		for _, node := range stage {
			if node == nil {
				continue
			}

			// Composition is declared here and nowhere else: this loop is the
			// only place that knows both which ring a node belongs to and
			// which barrier it sits behind.
			if composed, ok := node.(Composed); ok {
				composed.Compose(workload.name, index)
			}

			group = append(group, NewConsumer(node, workload.buffer, &workload.headSeq))
		}

		if len(group) > 0 {
			options = append(options, disruptor.Options.NewHandlerGroup(group...))
		}
	}

	options = append(options, disruptor.Options.NewHandlerGroup(
		&completionConsumer[T]{workload: workload},
	))
	workload.channel, workload.err = disruptor.New(options...)

	if workload.err == nil {
		go workload.channel.Listen()
	}
}

func optionList[Option any](initial ...Option) []Option {
	return initial
}

type completionConsumer[T any] struct {
	workload *Workload[T]
}

func (consumer *completionConsumer[T]) Handle(lower, upper int64) {
	if consumer.workload.output == nil {
		consumer.workload.completed.Store(upper)
		return
	}

	if ingress, ok := consumer.workload.output.(Ingress[T]); ok {
		for sequence := lower; sequence <= upper; sequence++ {
			ingress.Push(
				consumer.workload.buffer[sequence&system.Cfg.Runtime.Workspace.Mask],
			)
		}

		consumer.workload.completed.Store(upper)
		return
	}

	for sequence := lower; sequence <= upper; sequence++ {
		consumer.workload.output.Step(
			consumer.workload.buffer[sequence&system.Cfg.Runtime.Workspace.Mask],
		)
	}

	consumer.workload.completed.Store(upper)
}

/*
Step submits one value and returns only after this ring's final handler group
has completed it. This makes Workload an honest Node: an enclosing disruptor's
barrier represents completion of the nested ring, not merely its enqueue.
*/
func (workload *Workload[T]) Step(payload T) T {
	sequence, committed := workload.commit(payload)

	if !committed {
		return payload
	}

	for workload.completed.Load() < sequence {
		select {
		case <-workload.ctx.Done():
			return payload
		default:
			runtime.Gosched()
		}
	}

	return workload.buffer[sequence&system.Cfg.Runtime.Workspace.Mask]
}

/* Push submits one value without waiting for its consumers. */
func (workload *Workload[T]) Push(payload T) {
	workload.commit(payload)
}

func (workload *Workload[T]) commit(payload T) (int64, bool) {
	if workload == nil || workload.status.Current() != READY || workload.channel == nil {
		return 0, false
	}

	select {
	case <-workload.ctx.Done():
		return 0, false
	default:
	}

	sequence := workload.channel.Reserve(1)
	workload.buffer[sequence&system.Cfg.Runtime.Workspace.Mask] = payload
	workload.advanceHead(sequence)
	workload.channel.Commit(sequence, sequence)

	return sequence, true
}

func (workload *Workload[T]) advanceHead(sequence int64) {
	for {
		current := workload.headSeq.Load()

		if sequence <= current || workload.headSeq.CompareAndSwap(current, sequence) {
			return
		}
	}
}

func (workload *Workload[T]) admit() {
	if workload != nil && workload.err == nil && workload.channel != nil {
		workload.status.Transition(READY)
	}
}

func (workload *Workload[T]) connect(output Node[T]) {
	workload.output = output
}

func (workload *Workload[T]) Close() error {
	if workload == nil {
		return nil
	}

	workload.status.Transition(DONE)

	if workload.channel == nil {
		workload.cancel()

		return workload.err
	}

	err := errors.Join(workload.err, workload.channel.Close())

	for workload.completed.Load() < workload.headSeq.Load() {
		runtime.Gosched()
	}

	workload.cancel()

	return err
}

func (workload *Workload[T]) Error() error {
	if workload == nil {
		return errors.New("runtime: workload is nil")
	}

	return workload.err
}

func (workload *Workload[T]) Status() *Status {
	if workload == nil {
		return nil
	}

	return workload.status
}
