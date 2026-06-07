package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
TestStressedFillNeverInsideTheTouch is the spread-integrity regression: a stressed
taker fill must never be better than the side touch, no matter where the last
print sits. The pre-fix behavior anchored at quote.Last and let calm buys fill at
the bid, deleting the spread from paper and replay economics.
*/
func TestStressedFillNeverInsideTheTouch(t *testing.T) {
	Convey("Given a calm full-coverage book and a last print on the bid", t, func() {
		viper.Set("trading.replay.execution_stress_enabled", true)
		viper.Set("trading.replay.depth_shortfall_stress", 8)
		perspectives.PublishRegime(types.RegimeNone)

		quote := Quote{
			Symbol:    "TEST/EUR",
			Bid:       99.9,
			Ask:       100.1,
			Last:      99.9, // last trade hit the bid
			UpdatedAt: time.Now().UTC(),
			Book: market.Book{
				Symbol: "TEST/EUR",
				Bids:   []market.BookLevel{{Price: 99.9, Qty: 50}},
				Asks:   []market.BookLevel{{Price: 100.1, Qty: 50}},
			},
		}

		Convey("A stressed buy fills at or above the ask", func() {
			fill, err := StressedSlippageFill(quote, trading.Buy, 1, SymbolStress{})

			So(err, ShouldBeNil)
			So(fill.Price, ShouldBeGreaterThanOrEqualTo, quote.Ask)
		})

		Convey("A stressed sell fills at or below the bid", func() {
			fill, err := StressedSlippageFill(quote, trading.Sell, 1, SymbolStress{})

			So(err, ShouldBeNil)
			So(fill.Price, ShouldBeLessThanOrEqualTo, quote.Bid)
		})

		Convey("A replay-stressed buy fills at or above the ask", func() {
			fill, err := StressedSlippageReplayFill(quote, trading.Buy, 1, nil)

			So(err, ShouldBeNil)
			So(fill.Price, ShouldBeGreaterThanOrEqualTo, quote.Ask)
		})
	})
}

func TestExecutionStressMultiplier(t *testing.T) {
	Convey("Given turbulent snapshot readings", t, func() {
		snapshots := []types.Measurement{
			{Category: types.CategoryTurbulent, SNR: 2},
			{Category: types.CategoryLaminar, SNR: 0.5},
		}

		multiplier := ExecutionStressMultiplier(snapshots)

		Convey("It should expand slippage above baseline", func() {
			So(multiplier, ShouldBeGreaterThan, 1)
		})
	})

	Convey("Given hostile symbol stress in a bearish regime", t, func() {
		perspectives.PublishRegime(types.RegimeBearish)

		multiplier := ExecutionStressFromSymbol(SymbolStress{
			FluidCategory: types.CategoryTurbulent,
			FluidSNR:      2,
		})

		Convey("It should expand slippage above baseline", func() {
			So(multiplier, ShouldBeGreaterThan, 1)
		})
	})
}


func BenchmarkExecutionStressMultiplier(b *testing.B) {
	snapshots := []types.Measurement{
		{Category: types.CategoryTurbulent, SNR: 2},
		{Category: types.CategoryLaminar, SNR: 0.5},
	}

	for b.Loop() {
		_ = ExecutionStressMultiplier(snapshots)
	}
}
