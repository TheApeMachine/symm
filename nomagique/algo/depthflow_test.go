package algo

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

func TestDepthflowPrimitive(t *testing.T) {
	Convey("Given the Depthflow primitive", t, func() {
		stream := nmtypes.NewStream(Depthflow(), types.Frame{})

		Convey("It accurately computes touch and deep imbalance", func() {
			input := types.Frame{}
			input.Put(SymbolTouchBidQty, 20.0)
			input.Put(SymbolTouchAskQty, 10.0)
			input.Put(SymbolDeepBidQty, 80.0)
			input.Put(SymbolDeepAskQty, 20.0)
			input.Put(nmtypes.EventTimeSec, 1_700_000_000)
			input.Put(nmtypes.EventTimeNsec, 0)

			output, err := stream.Step(input)

			So(err, ShouldBeNil)
			// Touch imbalance: (20-10)/(20+10) = 10/30 = 0.3333333333333333
			So(output.MustGet(SymbolTouchImbalance), ShouldAlmostEqual, 1.0/3.0, 1e-4)
			// Deep imbalance: (80-20)/(80+20) = 60/100 = 0.6
			So(output.MustGet(SymbolDeepImbalance), ShouldAlmostEqual, 0.6, 1e-4)
			// Loaded score: (1/3) * 0.6 = 0.2
			So(output.MustGet(SymbolLoadedScore), ShouldAlmostEqual, 0.2, 1e-4)
			// Spoof score: 0
			So(output.MustGet(SymbolSpoofScore), ShouldEqual, 0)
		})

		Convey("Adversarial: Zero quantities / balanced book", func() {
			input := types.Frame{}
			input.Put(SymbolTouchBidQty, 0.0)
			input.Put(SymbolTouchAskQty, 0.0)
			input.Put(SymbolDeepBidQty, 0.0)
			input.Put(SymbolDeepAskQty, 0.0)
			input.Put(nmtypes.EventTimeSec, 1_700_000_001)
			input.Put(nmtypes.EventTimeNsec, 0)

			output, err := stream.Step(input)

			So(err, ShouldBeNil)
			So(output.MustGet(SymbolTouchImbalance), ShouldEqual, 0)
			So(output.MustGet(SymbolDeepImbalance), ShouldEqual, 0)
			So(output.MustGet(SymbolNeutralScore), ShouldEqual, 1.0)
		})
	})
}

func BenchmarkDepthflowPrimitive(b *testing.B) {
	stream := nmtypes.NewStream(Depthflow(), types.Frame{})
	input := types.Frame{}
	input.Put(SymbolTouchBidQty, 20.0)
	input.Put(SymbolTouchAskQty, 10.0)
	input.Put(SymbolDeepBidQty, 80.0)
	input.Put(SymbolDeepAskQty, 20.0)
	input.Put(nmtypes.EventTimeSec, 1_700_000_000)
	input.Put(nmtypes.EventTimeNsec, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = stream.Step(input)
	}
}
