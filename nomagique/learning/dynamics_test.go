package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestPredictiveDynamics(t *testing.T) {
	Convey("Given an event-time predictive dynamics stream", t, func() {
		stream := types.NewStream(PredictiveDynamics, types.Frame{})
		first := types.Frame{}
		first.Put(SymbolDynamicsTime, 1)
		first.Put(SymbolDynamicsPosition, 2)
		first.Put(SymbolDynamicsActivity, 0.25)
		firstOutput := stream.Step(first)
		So(firstOutput.Err, ShouldBeNil)
		So(firstOutput.MustGet(SymbolDynamicsReady), ShouldEqual, 0)

		Convey("A later observation should expose motion, memory, energy, and jump diagnostics", func() {
			second := types.Frame{}
			second.Put(SymbolDynamicsTime, 2)
			second.Put(SymbolDynamicsPosition, 3)
			second.Put(SymbolDynamicsActivity, 0.75)
			second.Put(SymbolDynamicsExternalPower, 1)
			secondOutput := stream.Step(second)

			So(secondOutput.Err, ShouldBeNil)
			So(secondOutput.MustGet(SymbolDynamicsReady), ShouldEqual, 1)
			So(secondOutput.MustGet(SymbolDynamicsVelocity), ShouldEqual, 1)
			So(secondOutput.MustGet(SymbolDynamicsMemoryScale), ShouldBeGreaterThan, 0)
			So(secondOutput.MustGet(SymbolDynamicsStoredEnergy), ShouldBeGreaterThan, 0)
			So(secondOutput.MustGet(SymbolDynamicsContinuousVariance), ShouldBeGreaterThanOrEqualTo, 0)
			So(secondOutput.MustGet(SymbolDynamicsJumpVariance), ShouldBeGreaterThanOrEqualTo, 0)
			So(secondOutput.MustGet(SymbolDynamicsEquivarianceNorm), ShouldAlmostEqual, 1)
		})

		Convey("A regressed timestamp should leave committed state untouched", func() {
			before := stream.Project()
			regressed := types.Frame{}
			regressed.Put(SymbolDynamicsTime, 0.5)
			regressed.Put(SymbolDynamicsPosition, 4)
			output := stream.Step(regressed)

			So(output.Err, ShouldNotBeNil)
			projected := stream.Project()
			So(projected.Equal(&before), ShouldBeTrue)
		})
	})
}

func BenchmarkPredictiveDynamics(b *testing.B) {
	stream := types.NewStream(PredictiveDynamics, types.Frame{})
	initial := types.Frame{}
	initial.Put(SymbolDynamicsTime, 1)
	initial.Put(SymbolDynamicsPosition, 0)
	initial.Put(SymbolDynamicsActivity, 0)

	if output := stream.Step(initial); output.Err != nil {
		b.Fatal(output.Err)
	}

	input := types.Frame{}
	input.Put(SymbolDynamicsActivity, 0.5)
	input.Put(SymbolDynamicsExternalPower, 0.25)
	b.ReportAllocs()

	for iteration := 0; b.Loop(); iteration++ {
		input.Put(SymbolDynamicsTime, float64(iteration+2))
		input.Put(SymbolDynamicsPosition, float64(iteration%100)/100)

		if output := stream.Step(input); output.Err != nil {
			b.Fatal(output.Err)
		}
	}
}
