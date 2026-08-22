package depthflow

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestDepthFlowNumber(t *testing.T) {
	Convey("Given touch and deep book quantities on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		Convey("Loaded Book: when touch and deep imbalance align heavily on bid side", func() {
			input := nomagique.Frame{}
			input.Put(algo.SymbolTouchBidQty, 10.0)
			input.Put(algo.SymbolTouchAskQty, 2.0)
			input.Put(algo.SymbolDeepBidQty, 50.0)
			input.Put(algo.SymbolDeepAskQty, 10.0)
			input.Put(nmtypes.EventTimeSec, 1_700_001_000)
			input.Put(nmtypes.EventTimeNsec, 0)

			output, err := signal.number.Step("AAA/USD", input)

			So(err, ShouldBeNil)

			touchImbalance := output.MustGet(SymbolTouchImbalance)
			deepImbalance := output.MustGet(SymbolDeepImbalance)
			loadedScore := output.MustGet(SymbolLoadedScore)
			spoofScore := output.MustGet(SymbolSpoofScore)

			// Touch imbalance = (10-2)/(10+2) = 8/12 = 0.666...
			So(touchImbalance, ShouldAlmostEqual, 8.0/12.0, 1e-4)
			// Deep imbalance = (50-10)/(50+10) = 40/60 = 0.666...
			So(deepImbalance, ShouldAlmostEqual, 40.0/60.0, 1e-4)
			// Loaded score = 0.666 * 0.666 = 0.444...
			So(loadedScore, ShouldAlmostEqual, (8.0/12.0)*(40.0/60.0), 1e-4)
			// Spoof score = 0 because signs agree
			So(spoofScore, ShouldEqual, 0)
		})

		Convey("Spoofed Book: when touch is heavily bid but deep book is heavily ask", func() {
			input := nomagique.Frame{}
			input.Put(algo.SymbolTouchBidQty, 10.0)
			input.Put(algo.SymbolTouchAskQty, 1.0)
			input.Put(algo.SymbolDeepBidQty, 2.0)
			input.Put(algo.SymbolDeepAskQty, 40.0)
			input.Put(nmtypes.EventTimeSec, 1_700_001_001)
			input.Put(nmtypes.EventTimeNsec, 0)

			output, err := signal.number.Step("AAA/USD", input)

			So(err, ShouldBeNil)

			touchImbalance := output.MustGet(SymbolTouchImbalance)
			deepImbalance := output.MustGet(SymbolDeepImbalance)
			spoofScore := output.MustGet(SymbolSpoofScore)
			loadedScore := output.MustGet(SymbolLoadedScore)

			// Touch imbalance = (10-1)/11 = 9/11 > 0
			So(touchImbalance, ShouldBeGreaterThan, 0)
			// Deep imbalance = (2-40)/42 = -38/42 < 0
			So(deepImbalance, ShouldBeLessThan, 0)
			// Spoof score > 0
			So(spoofScore, ShouldBeGreaterThan, 0)
			// Loaded score = 0
			So(loadedScore, ShouldEqual, 0)
		})

		Convey("Adversarial: Empty book / zero quantities produces stable zeros without panic", func() {
			input := nomagique.Frame{}
			input.Put(algo.SymbolTouchBidQty, 0.0)
			input.Put(algo.SymbolTouchAskQty, 0.0)
			input.Put(algo.SymbolDeepBidQty, 0.0)
			input.Put(algo.SymbolDeepAskQty, 0.0)
			input.Put(nmtypes.EventTimeSec, 1_700_001_002)
			input.Put(nmtypes.EventTimeNsec, 0)

			output, err := signal.number.Step("AAA/USD", input)

			So(err, ShouldBeNil)
			So(output.MustGet(SymbolTouchImbalance), ShouldEqual, 0)
			So(output.MustGet(SymbolDeepImbalance), ShouldEqual, 0)
			So(output.MustGet(SymbolSpoofScore), ShouldEqual, 0)
			So(output.MustGet(SymbolLoadedScore), ShouldEqual, 0)
		})
	})
}

func BenchmarkDepthFlowPipeline(b *testing.B) {
	thesis := types.NewThesis(context.Background(), nil)
	signal := NewSignal(context.Background(), thesis)
	defer signal.Close()

	input := nomagique.Frame{}
	input.Put(algo.SymbolTouchBidQty, 10.0)
	input.Put(algo.SymbolTouchAskQty, 2.0)
	input.Put(algo.SymbolDeepBidQty, 50.0)
	input.Put(algo.SymbolDeepAskQty, 10.0)
	input.Put(nmtypes.EventTimeSec, 1_700_001_000)
	input.Put(nmtypes.EventTimeNsec, 0)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = signal.number.Step("AAA/USD", input)
	}
}
