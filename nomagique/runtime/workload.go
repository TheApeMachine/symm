package runtime

import (
	"context"
	"errors"
	"runtime"
	"slices"
	"sync/atomic"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
)

/*
Workload owns one disruptor whose handler groups are declared as Node stages.

Nodes in one stage run concurrently. The disruptor barrier completes the whole
stage before the next stage advances. Workload also implements Node, allowing
the same staged composition to be nested in a Workspace.
*/
type Workload[T any] struct {
	*System
	channel    disruptor.Disruptor
	buffer     []T
	headSeq    atomic.Int64
	completed  atomic.Int64
	children   []*Workload[T]
	required   func(T) bool
	admitted   atomic.Bool
	closed     atomic.Bool
	publishers atomic.Int64
	listening  chan struct{}
	finished   chan struct{}
	closeErr   error
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
	workload := &Workload[T]{
		System:    NewSystem(ctx, name),
		buffer:    make([]T, system.Cfg.Runtime.Workspace.Buffer),
		listening: make(chan struct{}),
		finished:  make(chan struct{}),
	}

	workload.headSeq.Store(-1)
	workload.completed.Store(-1)
	workload.Transition(WAITING)
	workload.start(stages)

	return workload
}

func (workload *Workload[T]) start(stages [][]Node[T]) {
	if len(stages) == 0 {
		workload.err = errnie.Error(errnie.Err(errnie.Validation, "workload: at least one stage required", nil))
		return
	}
	options := optionList(
		disruptor.Options.BufferCapacity(system.Cfg.Runtime.Workspace.Buffer),
		disruptor.Options.WriterCount(2), // Public ingress permits concurrent producers.
	)

	for index, stage := range stages {
		if len(stage) == 0 {
			workload.err = errnie.Error(errnie.Err(errnie.Validation, "workload: empty stage", nil))
			return
		}
		group := make([]disruptor.Handler, 0, len(stage))

		for _, node := range stage {
			if node == nil {
				workload.err = errnie.Error(errnie.Err(errnie.Validation, "workload: nil stage node", nil))
				return
			}
			child, nested := node.(*Workload[T])

			if workspace, ok := node.(*Workspace[T]); ok {
				child, nested = workspace.Workload, true
			}

			if nested {
				if slices.Contains(workload.children, child) {
					workload.err = errnie.Error(errnie.Err(errnie.Validation, "workload: duplicate child", nil))
					return
				}
				workload.children = append(workload.children, child)
				workload.err = errors.Join(workload.err, child.Error())
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
		workload,
	))

	if workload.err != nil {
		return
	}
	workload.channel, workload.err = disruptor.New(options...)

	if workload.err == nil {
		go func() {
			workload.channel.Listen()
			close(workload.listening)
		}()
	}
}

func optionList[Option any](initial ...Option) []Option {
	return initial
}

// Handle records completion after all declared handler groups have returned.
func (workload *Workload[T]) Handle(lower, upper int64) {
	var empty T

	for sequence := lower; sequence <= upper; sequence++ {
		workload.buffer[sequence&system.Cfg.Runtime.Workspace.Mask] = empty
	}
	workload.completed.Store(upper)
}

// Require declares the data readiness needed to process an observation.
// Configure it before admission. Unready observations produce no inner steps.
func (workload *Workload[T]) Require(ready func(T) bool) {
	workload.required = ready
}

/*
Step submits one value and returns only after this ring's final handler group
has completed it. Returning releases the enclosing stage barrier. Cancellation
does not release an observation that is already being processed.
*/
func (workload *Workload[T]) Step(payload T) T {
	sequence, committed := workload.commit(payload, false)

	if !committed {
		return payload
	}

	for workload.completed.Load() < sequence {
		runtime.Gosched()
	}

	return payload
}

/* Push submits one value without waiting for its consumers. */
func (workload *Workload[T]) Push(payload T) {
	workload.commit(payload, true)
}

func (workload *Workload[T]) commit(payload T, interruptible bool) (int64, bool) {
	if workload == nil || !workload.admitted.Load() || workload.closed.Load() || workload.channel == nil {
		return 0, false
	}

	workload.publishers.Add(1)
	defer workload.publishers.Add(-1)

	if workload.closed.Load() || (interruptible && workload.ctx.Err() != nil) {
		return 0, false
	}

	if workload.required != nil && !workload.required(payload) {
		workload.status.Transition(WAITING)
		return 0, false
	}
	workload.status.Transition(READY)
	sequence := workload.channel.TryReserve(1)

	for sequence == disruptor.ErrCapacityUnavailable {
		if workload.closed.Load() || (interruptible && workload.ctx.Err() != nil) {
			return 0, false
		}
		runtime.Gosched()
		sequence = workload.channel.TryReserve(1)
	}

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

// Admit opens children before the ring that publishes to them.
func (workload *Workload[T]) Admit() {
	if workload == nil || workload.err != nil || workload.closed.Load() {
		return
	}

	for _, child := range workload.children {
		child.Admit()
	}
	workload.admitted.Store(true)
	workload.status.Transition(READY)
}

// Close stops publication, drains accepted work, then closes the nested rings.
func (workload *Workload[T]) Close() error {
	if workload == nil {
		return nil
	}

	if !workload.closed.CompareAndSwap(false, true) {
		<-workload.finished
		return workload.closeErr
	}

	for workload.publishers.Load() != 0 {
		runtime.Gosched()
	}
	workload.closeErr = workload.err

	if workload.channel != nil {
		workload.closeErr = errors.Join(workload.closeErr, workload.channel.Close())
		<-workload.listening
	}

	for _, child := range workload.children {
		workload.closeErr = errors.Join(workload.closeErr, child.Close())
	}
	workload.cancel()
	workload.status.Transition(DONE)
	close(workload.finished)
	return workload.closeErr
}
