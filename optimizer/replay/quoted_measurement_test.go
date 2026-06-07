package replay

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestQuotedMeasurementHonestQuote(t *testing.T) {
	Convey("Given a capture-style row with wide spread and no book", t, func() {
		measurement := QuotedMeasurement(types.Measurement{
			Symbol:    "PUMP/EUR",
			Last:      1,
			SpreadBPS: 800,
		})

		Convey("It should derive bid/ask from spread without inventing depth", func() {
			So(measurement.Bid, ShouldBeGreaterThan, 0)
			So(measurement.Ask, ShouldBeGreaterThan, measurement.Bid)
			So(measurement.HasBookDepth(), ShouldBeFalse)
		})
	})

	Convey("Given a row with only last price", t, func() {
		measurement := QuotedMeasurement(types.Measurement{
			Symbol: "PUMP/EUR",
			Last:   1,
		})

		Convey("It should leave the quote incomplete", func() {
			So(measurement.Bid, ShouldEqual, 0)
			So(measurement.Ask, ShouldEqual, 0)
			So(measurement.HasBookDepth(), ShouldBeFalse)
		})
	})
}

func TestQuotedMeasurementWideSpreadBlocksPreflight(t *testing.T) {
	Convey("Given replay entry on a wide-spread microcap without book", t, func() {
		testconfig.Load(t)
		ledger := newReplayLedger(ReplayCosts{
			StartingCapital:  200,
			PositionFraction: 1,
			WalletCurrency:   "EUR",
			WalletBalances:   map[string]float64{"EUR": 200},
		})
		measurement := QuotedMeasurement(types.Measurement{
			Symbol:    "PUMP/EUR",
			Last:      1,
			SpreadBPS: 800,
		})

		ledger.openEntry(
			"PUMP/EUR",
			trading.Buy,
			reasoning.Act{Type: reasoning.ActionMarket},
			measurement,
			nil,
			0,
			measurement.At,
			0,
		)

		Convey("It should block at preflight like live trading", func() {
			So(ledger.holding("PUMP/EUR"), ShouldBeFalse)
			So(ledger.preflightBlocked, ShouldEqual, 1)
		})
	})
}
