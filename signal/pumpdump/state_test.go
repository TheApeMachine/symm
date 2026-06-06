package pumpdump

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestPumpStateRatioScale(t *testing.T) {
	Convey("Given a pump state ratio scale baseline", t, func() {
		state := newPumpState(adaptive.NewClassifier(
			[]float64{-0.10, 0.50, 2.00},
			[]float64{0, 1, 2, 3},
			[]string{"faded_exhaustion", "organic_trend", "coiled_compression", "vertical_ignition"},
		), time.Minute)

		Convey("When the EMA has no prior sample", func() {
			_, err := state.ratioScale(2, state.volBase)

			Convey("It should report baseline unobserved", func() {
				So(errors.Is(err, errBaselineUnobserved), ShouldBeTrue)
			})
		})

		Convey("When the EMA has a prior sample", func() {
			_, _ = state.ratioScale(2, state.volBase)
			scaled, err := state.ratioScale(4, state.volBase)

			Convey("It should return the ratio to the running norm", func() {
				So(err, ShouldBeNil)
				So(scaled, ShouldBeGreaterThan, 1)
			})
		})

		Convey("When the signed input is negative", func() {
			_, _ = state.ratioScale(2, state.moveBase)
			scaled, err := state.ratioScale(-1, state.moveBase)

			Convey("It should preserve the sign against the magnitude norm", func() {
				So(err, ShouldBeNil)
				So(scaled, ShouldBeLessThan, 0)
			})
		})
	})
}

func TestPumpStateFoldSignedVolume(t *testing.T) {
	Convey("Given a pump state folding buy and sell trades", t, func() {
		state := newPumpState(adaptive.NewClassifier(
			[]float64{-0.10, 0.50, 2.00},
			[]float64{0, 1, 2, 3},
			[]string{"faded_exhaustion", "organic_trend", "coiled_compression", "vertical_ignition"},
		), time.Minute)

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		_, _ = state.fold(market.TradeUpdate{
			Symbol: "ALT/EUR", Side: "buy", Price: 10, Qty: 4, Timestamp: base,
		})

		_, _ = state.fold(market.TradeUpdate{
			Symbol:    "ALT/EUR",
			Side:      "sell",
			Price:     10,
			Qty:       1,
			Timestamp: base.Add(time.Millisecond),
		})

		Convey("It should accumulate signed volume separately from gross", func() {
			So(state.signed.Sum(), ShouldEqual, 3)
			So(state.gross.Sum(), ShouldEqual, 5)
		})
	})
}
