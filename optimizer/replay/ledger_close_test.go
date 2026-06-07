package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestReplayPreemptUsesVictimSymbolPrice(t *testing.T) {
	Convey("Given a held position preempted by a higher-conviction entry on another symbol", t, func() {
		costs := ReplayCosts{
			StartingCapital:  200,
			PositionFraction: 0.5,
			WalletCurrency:   "EUR",
			WalletBalances:   map[string]float64{"EUR": 200},
		}
		ledger := newReplayLedger(costs)
		base := time.Unix(1_700_000_000, 0)

		ledger.openLong("BTC/EUR", 100, 0, base)
		So(ledger.holding("BTC/EUR"), ShouldBeTrue)

		ledger.observePrice(types.Measurement{Symbol: "BTC/EUR", Last: 100})
		ledger.preemptOpenPosition(
			"BTC/EUR",
			types.Measurement{Symbol: "ETH/EUR", Last: 50_000, At: base.Add(time.Second)},
			nil,
		)

		Convey("It should realize P&L from the victim's price, not the preempting symbol", func() {
			So(ledger.holding("BTC/EUR"), ShouldBeFalse)
			So(ledger.realized, ShouldAlmostEqual, 0, 1e-2)
			So(ledger.walletCash("EUR"), ShouldAlmostEqual, 200, 1e-2)
		})
	})
}

func TestReplayExitWithoutFreshQuoteBlocksAfterPriceMove(t *testing.T) {
	Convey("Given a held position whose price moved without a new quoted row", t, func() {
		testconfig.Load(t)
		ledger := newReplayLedger(ReplayCosts{
			StartingCapital:  200,
			PositionFraction: 1,
			WalletCurrency:   "EUR",
			WalletBalances:   map[string]float64{"EUR": 200},
		})
		base := time.Unix(1_700_000_000, 0)

		ledger.openLong("BTC/EUR", 100, 0, base)
		ledger.observeSymbolPrice("BTC/EUR", 110)

		ledger.applyStressed(
			reasoning.Act{Type: reasoning.ActionSettlePosition},
			types.Measurement{Symbol: "BTC/EUR", Last: 110, At: base.Add(time.Second)},
			nil,
			0,
		)

		Convey("It should refuse to settle on stale book", func() {
			So(ledger.holding("BTC/EUR"), ShouldBeTrue)
			So(ledger.exitBlocked, ShouldEqual, 1)
		})
	})
}

func TestReplayPreemptFreesCapitalForHigherConvictionEntry(t *testing.T) {
	Convey("Given entry preemption enabled", t, func() {
		costs := ReplayCosts{
			StartingCapital:  200,
			PositionFraction: 1,
			WalletCurrency:   "EUR",
			WalletBalances:   map[string]float64{"EUR": 200},
		}
		ledger := newReplayLedger(costs)
		base := time.Unix(1_700_000_000, 0)

		ledger.openLong("BTC/EUR", 100, 0, base)
		ledger.observePrice(types.Measurement{Symbol: "BTC/EUR", Last: 100})
		ledger.entryConviction["BTC/EUR"] = 1
		ledger.entryBatch = []batchedReplayEntry{{
			act:         reasoning.Act{Type: reasoning.ActionMarket},
			measurement: TradeableRowWithSignal("ETH/EUR", 50, base, 5, 1),
			conviction:  5,
			at:          base,
		}}
		ledger.entryBatchDeadline = base.Add(-time.Millisecond)
		ledger.flushEntryBatch(base)

		Convey("It should rotate capital without phantom cross-symbol P&L", func() {
			So(ledger.realized, ShouldAlmostEqual, 0, 1e-2)
			So(ledger.holding("ETH/EUR"), ShouldBeTrue)
			So(ledger.walletCash("EUR"), ShouldBeLessThan, 200)
		})
	})
}
