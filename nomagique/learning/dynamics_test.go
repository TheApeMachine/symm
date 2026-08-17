package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
)

func TestPredictiveDynamics(t *testing.T) {
	Convey("Given an event-time predictive dynamics stream", t, func() {
		stream := nomagique.NewStream(PredictiveDynamics, nomagique.Frame{})
		first := nomagique.Frame{}
		first.Put(SymbolDynamicsTime, 1)
		first.Put(SymbolDynamicsPosition, 2)
		first.Put(SymbolDynamicsActivity, 0.25)
		firstOutput, err := stream.Step(first)
		So(err, ShouldBeNil)
		So(firstOutput.MustGet(SymbolDynamicsReady), ShouldEqual, 0)

		Convey("A later observation should expose motion, memory, energy, and jump diagnostics", func() {
			second := nomagique.Frame{}
			second.Put(SymbolDynamicsTime, 2)
			second.Put(SymbolDynamicsPosition, 3)
			second.Put(SymbolDynamicsActivity, 0.75)
			second.Put(SymbolDynamicsExternalPower, 1)
			secondOutput, measureErr := stream.Step(second)

			So(measureErr, ShouldBeNil)
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
			regressed := nomagique.Frame{}
			regressed.Put(SymbolDynamicsTime, 0.5)
			regressed.Put(SymbolDynamicsPosition, 4)
			_, measureErr := stream.Step(regressed)

			So(measureErr, ShouldNotBeNil)
			So(stream.Project().Equal(before), ShouldBeTrue)
		})
	})
}

func BenchmarkPredictiveDynamics(b *testing.B) {
	stream := nomagique.NewStream(PredictiveDynamics, nomagique.Frame{})
	initial := nomagique.Frame{}
	initial.Put(SymbolDynamicsTime, 1)
	initial.Put(SymbolDynamicsPosition, 0)
	initial.Put(SymbolDynamicsActivity, 0)

	if _, err := stream.Step(initial); err != nil {
		b.Fatal(err)
	}

	input := nomagique.Frame{}
	input.Put(SymbolDynamicsActivity, 0.5)
	input.Put(SymbolDynamicsExternalPower, 0.25)
	b.ReportAllocs()

	for iteration := 0; iteration < b.N; iteration++ {
		input.Put(SymbolDynamicsTime, float64(iteration+2))
		input.Put(SymbolDynamicsPosition, float64(iteration%100)/100)

		if _, err := stream.Step(input); err != nil {
			b.Fatal(err)
		}
	}
}
