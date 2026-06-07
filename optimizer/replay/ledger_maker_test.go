package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestAdvanceMakerQueuesUsesBookDepletion(t *testing.T) {
	Convey("Given a resting buy limit with queue ahead on the tape book", t, func() {
		testconfig.Load(t)
		rules := broker.NewInstrumentRulesCache(t.Context())
		rules.InstallPairForTest(market.InstrumentPair{
			Symbol:         "BTC/EUR",
			PriceIncrement: 0.01,
			QtyIncrement:   0.0001,
			QtyMin:         0.0001,
			CostMin:        1,
		})
		costs := ReplayCosts{
			StartingCapital:  200,
			PositionFraction: 1,
			WalletCurrency:   "EUR",
			WalletBalances:   map[string]float64{"EUR": 200},
			InstrumentRules:  rules,
		}
		ledger := newReplayLedger(costs)
		base := time.Unix(1_700_000_000, 0).UTC()

		first := types.Measurement{
			Symbol: "BTC/EUR",
			Last:   100,
			Bid:    100,
			Ask:    101,
			At:     base,
			BookBids: []types.BookLevel{
				{Price: 100, Qty: 2},
			},
			BookAsks: []types.BookLevel{
				{Price: 101, Qty: 10},
			},
		}
		second := first
		second.At = base.Add(time.Second)
		second.Last = 99.5
		second.BookBids = []types.BookLevel{{Price: 100, Qty: 0}}

		ledger.queueMakerEntry(
			"BTC/EUR",
			trading.Buy,
			100,
			1,
			0,
			1,
			first,
			"test_setup",
		)
		ledger.observePrice(first)
		ledger.advanceMakerQueues(second)
		ledger.observePrice(second)
		ledger.advanceMakerQueues(second)

		Convey("It should fill once book depletion clears queue ahead", func() {
			So(ledger.holding("BTC/EUR"), ShouldBeTrue)
		})
	})
}

func TestQueueMakerEntryRequiresBook(t *testing.T) {
	Convey("Given a maker entry without book depth", t, func() {
		rules := broker.NewInstrumentRulesCache(t.Context())
		rules.InstallPairForTest(market.InstrumentPair{
			Symbol:         "BTC/EUR",
			PriceIncrement: 0.01,
		})
		ledger := newReplayLedger(ReplayCosts{
			StartingCapital:  200,
			PositionFraction: 1,
			WalletCurrency:   "EUR",
			WalletBalances:   map[string]float64{"EUR": 200},
			InstrumentRules:  rules,
		})

		ledger.queueMakerEntry(
			"BTC/EUR",
			trading.Buy,
			100,
			1,
			0,
			1,
			types.Measurement{Symbol: "BTC/EUR", Last: 100},
			"test_setup",
		)

		Convey("It should refuse to queue the order", func() {
			So(len(ledger.pendingMakers), ShouldEqual, 0)
			So(ledger.preflightBlocked, ShouldEqual, 1)
		})
	})
}
