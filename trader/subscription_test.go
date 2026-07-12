package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestNewSubscriptionPlan(t *testing.T) {
	Convey("Given an unsorted eligible instrument universe", t, func() {
		pairs := []kraken.InstrumentPair{
			{Symbol: "SOL/USD"},
			{Symbol: "BTC/USD"},
			{Symbol: "ETH/USD"},
		}

		Convey("When universe-wide observation batches are built", func() {
			plan, err := NewSubscriptionPlan(pairs, 2)

			Convey("Then every identity is observed before a heavy tier exists", func() {
				So(err, ShouldBeNil)
				So(plan.Observation(), ShouldResemble, [][]string{{"BTC/USD", "ETH/USD"}, {"SOL/USD"}})
				So(plan.Symbols(), ShouldResemble, []string{"BTC/USD", "ETH/USD", "SOL/USD"})
				So(plan.Trading(), ShouldBeEmpty)
				So(plan.Ranked(), ShouldBeFalse)
			})
		})

		Convey("When the configured batch size is invalid", func() {
			_, err := NewSubscriptionPlan(pairs, 0)

			Convey("Then plan construction fails visibly", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestSubscriptionPlanRank(t *testing.T) {
	Convey("Given a complete ticker row for every observed identity", t, func() {
		plan, err := NewSubscriptionPlan([]kraken.InstrumentPair{
			{Symbol: "BTC/USD"},
			{Symbol: "ETH/USD"},
			{Symbol: "SOL/USD"},
		}, 2)
		So(err, ShouldBeNil)

		tickers := []kraken.TickerData{
			liquidityTicker("SOL/USD", 99.99, 100.01, 1, 1),
			liquidityTicker("BTC/USD", 90, 110, 10, 10),
			liquidityTicker("ETH/USD", 99, 101, 5, 6),
		}

		Convey("When a bounded heavy tier is ranked", func() {
			err := plan.Rank(tickers, 1)

			Convey("Then maximin liquidity, rather than lexical order, selects the tier", func() {
				So(err, ShouldBeNil)
				So(plan.Ranked(), ShouldBeTrue)
				So(plan.Trading(), ShouldResemble, [][]string{{"ETH/USD"}})
			})
		})
	})

	Convey("Given one missing observed identity", t, func() {
		plan, err := NewSubscriptionPlan([]kraken.InstrumentPair{
			{Symbol: "BTC/USD"},
			{Symbol: "ETH/USD"},
		}, 2)
		So(err, ShouldBeNil)

		Convey("When ranking is attempted", func() {
			err := plan.Rank([]kraken.TickerData{
				liquidityTicker("BTC/USD", 99, 101, 1, 1),
			}, 1)

			Convey("Then the missing identity prevents a partial-universe tier", func() {
				So(err, ShouldNotBeNil)
				So(plan.Ranked(), ShouldBeFalse)
				So(plan.Trading(), ShouldBeEmpty)
			})
		})
	})
}

func BenchmarkNewSubscriptionPlan(b *testing.B) {
	pairs := make([]kraken.InstrumentPair, 1455)

	for index := range pairs {
		pairs[index].Symbol = string(rune(index+1)) + "/USD"
	}

	for b.Loop() {
		_, err := NewSubscriptionPlan(pairs, 50)

		if err != nil {
			b.Fatal(err)
		}
	}
}
