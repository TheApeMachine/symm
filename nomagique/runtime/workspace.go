package runtime

import (
	"context"
	"strconv"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime/disruptor"
)

/*
Workspace owns one LMAX ring and the ordered handler groups mounted on it.
Steps within one group run concurrently against the same sequence; a group
cannot advance onto a sequence until every handler in the group before it has
passed that sequence. Those are the ring's own barriers, so the runtime adds
no mutex, no per-event channel and no secondary scheduler to order them.

A Workspace is itself a Step, so a Workspace mounted in another Workspace's
stage is a ring of rings, ordered by the same grammar as any other stage.

A Workspace begins WAITING. Nothing may be published until Admit opens it,
which is how a partially constructed subscription universe is kept from
becoming input.
*/
type Workspace struct {
	*System
	ring      disruptor.Disruptor
	slots     []Slot
	mask      int64
	consumers []*Consumer
}

/*
NewWorkspace builds a ring whose capacity is the declared number of slots and
whose stages are mounted in the order given. Capacity must be a power of two,
because the ring addresses a slot by masking a sequence rather than dividing
by it.
*/
func NewWorkspace(
	ctx context.Context,
	name string,
	capacity uint32,
	writers uint8,
	stages [][]Step,
) (*Workspace, error) {
	if capacity == 0 || capacity&(capacity-1) != 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[runtime.workspace] ring capacity must be a power of two",
			nil,
		))
	}

	if len(stages) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[runtime.workspace] a ring carries no stages",
			nil,
		))
	}

	workspace := &Workspace{
		System: NewSystem(ctx, name),
		slots:  make([]Slot, capacity),
		mask:   int64(capacity) - 1,
	}

	handlerGroups := make([][]disruptor.Handler, 0, len(stages))

	for index, stage := range stages {
		if len(stage) == 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"[runtime.workspace] a stage carries no steps",
				nil,
			))
		}

		group := make([]disruptor.Handler, 0, len(stage))

		for position, step := range stage {
			consumer := NewConsumer(
				ctx,
				stageName(name, index, position),
				step,
				workspace.slots,
			)

			workspace.consumers = append(workspace.consumers, consumer)
			group = append(group, consumer)
		}

		handlerGroups = append(handlerGroups, group)
	}

	settings := disruptor.NewOptions(
		disruptor.Options.BufferCapacity(capacity),
		disruptor.Options.WriterCount(writers),
	)

	for _, group := range handlerGroups {
		settings = append(settings, disruptor.Options.NewHandlerGroup(group...))
	}

	ring, err := disruptor.New(settings...)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[runtime.workspace] failed to build ring",
			err,
		))
	}

	workspace.ring = ring
	workspace.AddCloser(ring)
	workspace.Transition(WAITING)

	return workspace, nil
}

/*
Admit opens the ring. Until it is called the Workspace is WAITING and refuses
publication, so a subscription universe that is still being constructed cannot
reach the stages mounted on it.
*/
func (workspace *Workspace) Admit() {
	if workspace.Status() == READY {
		return
	}

	go workspace.ring.Listen()
	workspace.Transition(READY)
}

/*
Push commits one observation and returns, so a source reader is governed by
the ring's own backpressure rather than by the cost of the stages behind it.
*/
func (workspace *Workspace) Push(payload []byte) error {
	select {
	case <-workspace.Context().Done():
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[runtime.workspace] context ended before publication",
			workspace.Context().Err(),
		))
	default:
	}

	if workspace.Status() != READY {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[runtime.workspace] ring has not been admitted",
			nil,
		))
	}

	sequence := workspace.ring.Reserve(1)
	workspace.slots[sequence&workspace.mask].Payload = payload
	workspace.ring.Commit(sequence, sequence)

	return nil
}

/*
Step publishes one observation into this ring, which is how a Workspace
mounted inside another ring's stage behaves as an ordinary stage.
*/
func (workspace *Workspace) Step(
	ctx context.Context, payload []byte,
) ([]byte, error) {
	if err := workspace.Push(payload); err != nil {
		return nil, err
	}

	return payload, nil
}

/*
Backlog reports the ring pressure a stage is under: what has been published
that this consumer has not yet passed. It is measured, never estimated.
*/
func (workspace *Workspace) Backlog() int64 {
	var stepped int64

	for _, consumer := range workspace.consumers {
		stepped += consumer.Stepped()
	}

	return stepped
}

/*
Consumers exposes the stages mounted on this ring so their progress can be
reported.
*/
func (workspace *Workspace) Consumers() []*Consumer { return workspace.consumers }

func stageName(ring string, stage, position int) string {
	return ring + ".stage" + strconv.Itoa(stage) + "." + strconv.Itoa(position)
}
