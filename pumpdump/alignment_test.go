package pumpdump

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestMeasureAlignmentPartialHints(t *testing.T) {
	Convey("Given only volume and book hints without price follow-through", t, func() {
		state := NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})
		state.lastPrice = 1
		state.imbalance = 0.7
		state.buyPressure = 0.4
		state.spreadBPS = 8
		_, _ = state.spreadCompression.Next(12, 8)

		confidence, err := state.measureAlignment(1.4)

		Convey("It should emit partial alignment below full breakout", func() {
			So(err, ShouldBeNil)
			So(confidence, ShouldBeGreaterThan, 0)
			So(confidence, ShouldBeLessThan, 0.5)
		})
	})
}

func TestMeasureAlignmentFullBreakout(t *testing.T) {
	Convey("Given aligned spike, book, spread, and price move", t, func() {
		state := NewPumpSymbol(asset.Pair{Wsname: "PUMP/EUR"})
		state.lastPrice = 1.003
		state.imbalance = 0.85
		state.buyPressure = 0.8
		state.spreadBPS = 6
		_, _ = state.spreadCompression.Next(12, 6)
		_, _ = state.volumeWindow.Next(0, 1_700_000_000, 50, 1)

		confidence, err := state.measureAlignment(2.5)

		Convey("It should emit stronger alignment without reaching certainty", func() {
			So(err, ShouldBeNil)
			So(confidence, ShouldBeGreaterThan, 0.3)
			So(confidence, ShouldBeLessThan, 1)
		})
	})
}
