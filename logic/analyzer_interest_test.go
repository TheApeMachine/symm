package logic_test

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestAnalyzerInterest proves inventory priority and Hawkes intensity ordering
from the production graph's actual wallet and fixture-driven arrival process.
*/
func TestAnalyzerInterest(t *testing.T) {
	Convey("Given one symbol receiving a faster simulated arrival stream", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})
		var thesis *types.Thesis
		So(market.Warmup(func() error {
			var err error
			thesis, err = wired.Crypto.Tick()
			return err
		}), ShouldBeNil)
		leader := market.Symbols[2]

		for range 4 {
			So(market.Apply(tests.MarketStep{
				Advance: time.Second / 4,
				Actions: []tests.MarketAction{
					{
						Kind:   tests.MarketTrade,
						Symbol: leader,
						Side:   "buy",
						Qty:    1,
					},
					{
						Kind:   tests.MarketRefill,
						Symbol: leader,
						Side:   "sell",
						Qty:    1,
					},
				},
			}, func() error {
				var err error
				thesis, err = wired.Crypto.Tick()
				return err
			}), ShouldBeNil)
		}

		interest := wired.Analyzer.Interest(thesis)

		Convey("Its measured arrival intensity should put that symbol first", func() {
			So(interest, ShouldHaveLength, len(market.Symbols))
			So(interest[0], ShouldEqual, leader)
		})

		Convey("An actual open wallet lot should remain ahead of that leader", func() {
			inventory := market.Symbols[1]
			position, err := wired.Desk.Buy(
				types.NewHolding(
					t.Context(),
					inventory,
					decimal.NewFromInt64(1),
				),
				false,
			)
			So(err, ShouldBeNil)
			So(market.Paper.Drain(), ShouldBeNil)
			So(position.Status(), ShouldEqual, types.OPEN)
			holding, err := wired.Balance.Holding(inventory)
			So(err, ShouldBeNil)
			thesis.Holdings.Store(inventory, &holding)

			interest = wired.Analyzer.Interest(thesis)
			So(interest, ShouldHaveLength, len(market.Symbols))
			So(interest[0], ShouldEqual, inventory)
			So(interest[1], ShouldEqual, leader)
		})
	})
}
