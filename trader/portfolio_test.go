package trader

import (
	"testing"

	"github.com/spf13/viper"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
)

func testPortfolio() *Portfolio {
	viper.Set("trading.slots.normal", 4)
	viper.Set("trading.entry.opportunity_slot_count", 2)
	viper.Set("trading.stop.trailing_offset_bps", 100)
	viper.Set("trading.stop.min_offset_bps", 20)
	viper.Set("trading.stop.max_offset_bps", 500)
	viper.Set("trading.paper.taker_fee_bps", 40)
	viper.Set("trading.paper.slippage_bps", 2)

	portfolio, err := NewPortfolio(nil)

	if err != nil {
		panic(err)
	}

	return portfolio
}

func buy(symbol string) *logic.Action {
	return &logic.Action{
		Symbol:   symbol,
		Side:     "buy",
		Score:    0.5,
		Fraction: 0.05,
		Price:    100,
	}
}

func sell(symbol string) *logic.Action {
	return &logic.Action{Symbol: symbol, Side: "sell", Score: 0.5, Price: 100}
}

func held(symbol string, returnPct float64) map[string]broker.PositionData {
	return map[string]broker.PositionData{
		symbol: {Symbol: symbol, ReturnPct: returnPct},
	}
}

func intentFor(intents []tradeIntent, symbol string) (tradeIntent, bool) {
	for _, intent := range intents {
		if intent.symbol == symbol {
			return intent, true
		}
	}

	return tradeIntent{}, false
}

func TestPortfolioTrailingStopExit(testingTB *testing.T) {
	Convey("Given a filled position that ran up then bled back", testingTB, func() {
		portfolio := testPortfolio()

		enter, _ := intentFor(portfolio.Reconcile([]*logic.Action{buy("TAO/USD")}, nil), "TAO/USD")
		So(enter.kind, ShouldEqual, intentEnter)

		// Fill confirmed at +2%; peak return is now 0.02.
		portfolio.Reconcile(nil, held("TAO/USD", 0.02))

		Convey("When it drops more than the trailing offset below its peak", func() {
			intents := portfolio.Reconcile(nil, held("TAO/USD", 0.005))

			Convey("Then a trailing-stop exit fires", func() {
				exit, ok := intentFor(intents, "TAO/USD")
				So(ok, ShouldBeTrue)
				So(exit.kind, ShouldEqual, intentExit)
				So(exit.reason, ShouldEqual, "trailing_stop")
			})
		})

		Convey("When it holds within the trailing offset", func() {
			intents := portfolio.Reconcile(nil, held("TAO/USD", 0.015))

			Convey("Then it keeps holding", func() {
				_, ok := intentFor(intents, "TAO/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPortfolioReversalExit(testingTB *testing.T) {
	Convey("Given a filled position in profit past round-trip friction", testingTB, func() {
		portfolio := testPortfolio()

		portfolio.Reconcile([]*logic.Action{buy("TAO/USD")}, nil)
		portfolio.Reconcile(nil, held("TAO/USD", 0.02))

		Convey("When the read reverses to down-conviction", func() {
			intents := portfolio.Reconcile([]*logic.Action{sell("TAO/USD")}, held("TAO/USD", 0.02))

			Convey("Then a thesis-reversal exit fires", func() {
				exit, ok := intentFor(intents, "TAO/USD")
				So(ok, ShouldBeTrue)
				So(exit.kind, ShouldEqual, intentExit)
				So(exit.reason, ShouldEqual, "thesis_reversal")
			})
		})

		Convey("When the reversal comes before friction is cleared", func() {
			portfolioEarly := testPortfolio()
			portfolioEarly.Reconcile([]*logic.Action{buy("SUI/USD")}, nil)
			portfolioEarly.Reconcile(nil, held("SUI/USD", 0.001))
			intents := portfolioEarly.Reconcile([]*logic.Action{sell("SUI/USD")}, held("SUI/USD", 0.001))

			Convey("Then it holds instead of paying fees to churn", func() {
				_, ok := intentFor(intents, "SUI/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPortfolioNoReentry(testingTB *testing.T) {
	Convey("Given a symbol already held", testingTB, func() {
		portfolio := testPortfolio()
		portfolio.Reconcile([]*logic.Action{buy("XRP/USD")}, nil)

		Convey("When another buy read arrives for the same symbol", func() {
			intents := portfolio.Reconcile([]*logic.Action{buy("XRP/USD")}, held("XRP/USD", 0.01))

			Convey("Then it does not stack a second position", func() {
				_, ok := intentFor(intents, "XRP/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}
