package temporal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestGovernor(t *testing.T) {
	Convey("Given a stability decline", t, func() {
		state := types.Frame{}
		state.Put(SymbolCapacity, 4)
		state.Put(SymbolPreviousStability, 0.75)
		input := types.Frame{}
		input.Put(nomagique.SampleCount, 4)
		input.Put(statistic.SymbolStability, 0.5)

		_, output, err := Governor(state, input)

		So(err, ShouldBeNil)
		So(output.MustGet(nmtypes.Span), ShouldEqual, 8.0)
	})

	Convey("Given perfect stability with unused capacity", t, func() {
		state := types.Frame{}
		state.Put(SymbolCapacity, 8)
		state.Put(SymbolPreviousStability, 0.75)
		input := types.Frame{}
		input.Put(nomagique.SampleCount, 4)
		input.Put(statistic.SymbolStability, 1)

		_, output, err := Governor(state, input)

		So(err, ShouldBeNil)
		So(output.MustGet(nmtypes.Span), ShouldEqual, 4.0)
	})
}

func BenchmarkGovernor(benchmark *testing.B) {
	state := types.Frame{}
	state.Put(SymbolCapacity, 8)
	state.Put(SymbolPreviousStability, 0.75)
	input := types.Frame{}
	input.Put(nomagique.SampleCount, 4)
	input.Put(statistic.SymbolStability, 0.5)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _, _ = Governor(state, input)
	}
}
