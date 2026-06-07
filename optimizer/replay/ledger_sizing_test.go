package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestReplayEntryQuantityMatchesDeployedSlot(t *testing.T) {
	Convey("Given a fill price far from the quote reference", t, func() {
		costs := ReplayCosts{
			StartingCapital:  200,
			PositionFraction: 1,
			WalletCurrency:   "EUR",
			WalletBalances:   map[string]float64{"EUR": 200},
		}
		ledger := newReplayLedger(costs)
		at := time.Unix(1_700_000_000, 0)
		entry := QuotedMeasurement(types.Measurement{
			Symbol: "PUMP/EUR",
			Last:   1e-8,
			Bid:    1e-8,
			Ask:    1e-8,
			At:     at,
		})

		ledger.openEntry(
			"PUMP/EUR",
			trading.Buy,
			reasoning.Act{Type: reasoning.ActionMarket},
			entry,
			nil,
			0,
			at,
			0,
		)

		Convey("It should not open a coin stack larger than the funded slot", func() {
			position, open := ledger.positions["PUMP/EUR"]

			if open {
				So(position.cost, ShouldBeLessThanOrEqualTo, 200+1e-6)
				So(position.quantity*position.entryPrice, ShouldBeLessThanOrEqualTo, 200+1e-6)
			}

			ledger.applyStressed(
				reasoning.Act{Type: reasoning.ActionSettlePosition},
				QuotedMeasurement(types.Measurement{
					Symbol: "PUMP/EUR",
					Last:   1,
					Bid:    1,
					Ask:    1,
					At:     at.Add(time.Second),
				}),
				nil,
				0,
			)

			So(ledger.realized, ShouldBeLessThan, 10_000)
			So(ledger.walletCash("EUR"), ShouldBeLessThan, 10_000)
		})
	})
}
