package runtime

import (
	"context"
	"errors"

	"github.com/smarty/go-disruptor"
	"github.com/theapemachine/symm/system"
)

/*
Workload owns one bounded ring whose handler groups are declared as Node stages.

Nodes in one stage run concurrently. The ring barrier completes the whole
stage before the next stage advances. Workload also implements Node, allowing
the same staged composition to be nested in a Workspace.
*/
type Workload[T any] struct {
	ctx    context.Context
	cancel context.CancelFunc
	name   string
	err    error
	status *Status
	ring   *ring[T]
	output Node[T]
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
	}
	workload.ring, workload.err = newRing[T](ctx, system.Cfg.Runtime.Workspace.Buffer)
	workload.status.Transition(WAITING)
	workload.start(stages, writers)

	return workload
}

func (workload *Workload[T]) start(stages [][]Node[T], writers uint8) {
	if workload.err != nil {
		return
	}

	if writers < 1 {
		workload.err = errors.New("runtime: workload requires at least one writer")

		return
	}

	groups := [][]disruptor.Handler{}

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

			group = append(group, NewConsumer(node, workload.ring.buffer, &workload.ring.head))
		}

		if len(group) > 0 {
			groups = append(groups, group)
		}
	}

	groups = append(groups, []disruptor.Handler{&completionConsumer[T]{workload: workload}})
	workload.ring.Start(groups)
}

type completionConsumer[T any] struct {
	workload *Workload[T]
}

func (consumer *completionConsumer[T]) Handle(lower, upper int64) {
	if consumer.workload.output == nil {
		return
	}

	if ingress, ok := consumer.workload.output.(Ingress[T]); ok {
		for sequence := lower; sequence <= upper; sequence++ {
			ingress.Push(
				consumer.workload.ring.buffer[sequence&system.Cfg.Runtime.Workspace.Mask],
			)
		}

		return
	}

	for sequence := lower; sequence <= upper; sequence++ {
		consumer.workload.output.Step(
			consumer.workload.ring.buffer[sequence&system.Cfg.Runtime.Workspace.Mask],
		)
	}
}

/*
Step submits one value and returns only after this ring's final handler group
has completed it. This makes Workload an honest Node: an enclosing ring's
barrier represents completion of the nested ring, not merely its enqueue.
*/
func (workload *Workload[T]) Step(payload T) T {
	sequence, committed := workload.commit(payload)

	if !committed {
		return payload
	}

	workload.ring.Await(sequence)
	return payload
}

/* Push submits one value without waiting for its consumers. */
func (workload *Workload[T]) Push(payload T) {
	workload.commit(payload)
}

func (workload *Workload[T]) commit(payload T) (int64, bool) {
	if workload == nil || workload.status.Current() != READY || workload.ring == nil {
		return 0, false
	}
	return workload.ring.Publish(payload)
}

func (workload *Workload[T]) admit() {
	if workload != nil && workload.err == nil && workload.ring != nil {
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

	if workload.ring != nil {
		workload.ring.Close()
	}
	workload.cancel()
	return workload.err
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
