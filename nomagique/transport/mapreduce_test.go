package transport_test

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

// MapReduce is map followed by reduction, not the former concurrent consumer
// registry. Queue/lifecycle semantics belong to the unconverted runtime.
func TestMapReduceNext(t *testing.T) {
	tests.CheckMapReduce(t,
		transport.NewMapReduce(
			calculus.NewSquare(transport.NewIO(core.From(0.0))),
			arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
		), "square-sum")
	tests.CheckMapReduce(t,
		transport.NewMapReduce(
			store.NewConstant(core.From(1.0)),
			arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
		), "count")
}
