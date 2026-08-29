package runtime

import "github.com/theapemachine/symm/system"

type Node[T any] interface {
	Step(T) T
}

/*
BoundarySlot* names the fixed, exclusive types.Envelope.Boundaries index one
Observe group in a Workload's declared graph stamps its "passed here at T"
witness into — the Nth Observe group in the graph always uses the Nth slot,
across every Workload (a Ticker envelope and a Level3 envelope never share
memory, so reusing slot identity across different Workloads is safe; what
must stay disjoint is concurrent writers within one HandlerGroup, and an
Observe group has none of the Compute mutation that would race against it).
*/
const (
	BoundarySlotAfterSignals = iota
	BoundarySlotAfterCategory
	BoundarySlotAfterLogic
	BoundarySlotAfterStrategy
)

type Consumer[T any] struct {
	node   Node[T]
	buffer []T
}

func NewConsumer[T any](node Node[T], buffer []T) *Consumer[T] {
	return &Consumer[T]{node: node, buffer: buffer}
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
	for seq := lower; seq <= upper; seq++ {
		consumer.node.Step(consumer.buffer[seq&system.Cfg.Runtime.Workspace.Mask])
	}
}
