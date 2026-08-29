package runtime

import (
	"sync/atomic"

	"github.com/theapemachine/symm/system"
)

type Node[T any] interface {
	Step(T) T
}

/*
BacklogStepper is a Node that also wants to know how many slots behind the
Workload's producer this call is running — real ring pressure, read from the
same sequence numbers the disruptor itself uses for backpressure, never
estimated. A node opts in by implementing StepBacklog in addition to Step;
Consumer favors StepBacklog when present and falls back to Step otherwise, so
every existing Node implementation is unaffected.
*/
type BacklogStepper[T any] interface {
	Node[T]
	StepBacklog(value T, backlog int64) T
}

type Consumer[T any] struct {
	node        Node[T]
	backlogNode BacklogStepper[T]
	buffer      []T
	headSeq     *atomic.Int64
}

func NewConsumer[T any](node Node[T], buffer []T, headSeq *atomic.Int64) *Consumer[T] {
	consumer := &Consumer[T]{node: node, buffer: buffer, headSeq: headSeq}
	consumer.backlogNode, _ = node.(BacklogStepper[T])

	return consumer
}

/*
Handle steps one node over every slot in [lower, upper]. It never writes the
Step return value back into the ring: every Handler in a HandlerGroup runs
concurrently against the same buffer, so two Handlers reassigning the same
slot pointer would race even though each only mutates its own field on the
shared envelope. Step still returns T so a Node can be used standalone
outside a group; Handle intentionally discards that return here.
*/
func (consumer *Consumer[T]) Handle(lower, upper int64) {
	if consumer.backlogNode != nil {
		for seq := lower; seq <= upper; seq++ {
			backlog := consumer.headSeq.Load() - seq
			consumer.backlogNode.StepBacklog(consumer.buffer[seq&system.Cfg.Runtime.Workspace.Mask], backlog)
		}

		return
	}

	for seq := lower; seq <= upper; seq++ {
		consumer.node.Step(consumer.buffer[seq&system.Cfg.Runtime.Workspace.Mask])
	}
}
