package algo

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestExhaustPrimitive(t *testing.T) {
	Convey("Given the Exhaust primitive", t, func() {
		stream := nmtypes.NewStream(Exhaust(), types.Frame{})

		Convey("Thermal Rejection: buy aggressor meeting adverse price drop", func() {
			input := types.Frame{}
			input.Put(statistic.SymbolVolume, 50.0)
			input.Put(statistic.SymbolSpread, 0.5)
			input.Put(statistic.SymbolPriceDelta, -2.0)
			input.Put(statistic.SymbolAggressorSide, 1.0)
			input.Put(nmtypes.EventTimeSec, 1_700_000_000)
			input.Put(nmtypes.EventTimeNsec, 0)

			output, err := stream.Step(input)

			So(err, ShouldBeNil)
			So(output.MustGet(statistic.SymbolThermal), ShouldBeGreaterThan, 0)
			So(output.MustGet(statistic.SymbolUrgency), ShouldBeGreaterThan, 0)
		})

		Convey("Reversal: Directional flip opposing previous flow", func() {
			// First tick: buy flow
			input1 := types.Frame{}
			input1.Put(statistic.SymbolVolume, 50.0)
			input1.Put(statistic.SymbolSpread, 0.5)
			input1.Put(statistic.SymbolPriceDelta, 1.0)
			input1.Put(statistic.SymbolAggressorSide, 1.0)
			input1.Put(nmtypes.EventTimeSec, 1_700_000_001)
			input1.Put(nmtypes.EventTimeNsec, 0)
			_, err := stream.Step(input1)
			So(err, ShouldBeNil)

			// Second tick: aggressive sell flow (reversal)
			input2 := types.Frame{}
			input2.Put(statistic.SymbolVolume, 80.0)
			input2.Put(statistic.SymbolSpread, 0.5)
			input2.Put(statistic.SymbolPriceDelta, -1.0)
			input2.Put(statistic.SymbolAggressorSide, -1.0)
			input2.Put(nmtypes.EventTimeSec, 1_700_000_002)
			input2.Put(nmtypes.EventTimeNsec, 0)
			output2, err := stream.Step(input2)

			So(err, ShouldBeNil)
			So(output2.MustGet(statistic.SymbolReversal), ShouldBeGreaterThan, 0)
			So(output2.MustGet(statistic.SymbolUrgency), ShouldBeGreaterThan, 0)
		})

		Convey("Adversarial: Zero volume and neutral ticks produce zero urgency", func() {
			input := types.Frame{}
			input.Put(statistic.SymbolVolume, 0.0)
			input.Put(statistic.SymbolSpread, 0.0)
			input.Put(statistic.SymbolPriceDelta, 0.0)
			input.Put(statistic.SymbolAggressorSide, 0.0)
			input.Put(nmtypes.EventTimeSec, 1_700_000_003)
			input.Put(nmtypes.EventTimeNsec, 0)

			output, err := stream.Step(input)

			So(err, ShouldBeNil)
			So(output.MustGet(statistic.SymbolUrgency), ShouldEqual, 0)
		})
	})
}

func BenchmarkExhaustPrimitive(b *testing.B) {
	stream := nmtypes.NewStream(Exhaust(), types.Frame{})
	input := types.Frame{}
	input.Put(statistic.SymbolVolume, 50.0)
	input.Put(statistic.SymbolSpread, 0.5)
	input.Put(statistic.SymbolPriceDelta, -2.0)
	input.Put(statistic.SymbolAggressorSide, 1.0)
	input.Put(nmtypes.EventTimeSec, 1_700_000_000)
	input.Put(nmtypes.EventTimeNsec, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = stream.Step(input)
	}
}
