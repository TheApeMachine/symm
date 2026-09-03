package temporal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestGovernor(t *testing.T) {
	Convey("Given a stability decline", t, func() {
		state := types.Frame{}
		state.Put(SymbolCapacity, 4)
		state.Put(SymbolPreviousStability, 0.75)
		input := types.Frame{}
		input.Put(types.SampleCount, 4)
		input.Put(types.MustIntern("stability"), 0.5)

		merged := state
		merged.Merge(&input)
		GovernorPrimitive(&merged)

		So(merged.Err, ShouldBeNil)
		So(merged.MustGet(nmtypes.Span), ShouldEqual, 8.0)
	})

	Convey("Given perfect stability with unused capacity", t, func() {
		state := types.Frame{}
		state.Put(SymbolCapacity, 8)
		state.Put(SymbolPreviousStability, 0.75)
		input := types.Frame{}
		input.Put(types.SampleCount, 4)
		input.Put(types.MustIntern("stability"), 1)

		merged := state
		merged.Merge(&input)
		GovernorPrimitive(&merged)

		So(merged.Err, ShouldBeNil)
		So(merged.MustGet(nmtypes.Span), ShouldEqual, 4.0)
	})
}

func BenchmarkGovernor(benchmark *testing.B) {
	state := types.Frame{}
	state.Put(SymbolCapacity, 8)
	state.Put(SymbolPreviousStability, 0.75)
	input := types.Frame{}
	input.Put(types.SampleCount, 4)
	input.Put(types.MustIntern("stability"), 0.5)
	benchmark.ReportAllocs()

	for benchmark.Loop() {
		merged := state
		merged.Merge(&input)
		GovernorPrimitive(&merged)
	}
}
