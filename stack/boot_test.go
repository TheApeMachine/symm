package stack_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestBooter_Test proves the injected paper Conn closes the real production
Desk, Position, execution, and Balance loop against the simulated market.
*/
func TestBooter_Test(t *testing.T) {
	Convey("Given the complete production graph on a simulated market", t, func() {
		market := tests.NewMarket(t.Context(), 1)
		So(market.Bootstrap(), ShouldBeNil)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() { wired.Close(); market.Close() })
		So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
		symbol := market.Symbols[0]
		quantity := decimal.NewFromInt64(1)

		Convey("A Desk entry fills at the simulated ask and updates inventory", func() {
			position, err := wired.Desk.Buy(
				*types.NewHolding(t.Context(), symbol, quantity),
				true,
			)
			So(err, ShouldBeNil)
			So(position.Status(), ShouldEqual, types.PENDING)
			So(market.Paper.Drain(), ShouldBeNil)
			So(position.Status(), ShouldEqual, types.OPEN)
			holding, err := wired.Balance.Holding(symbol)
			So(err, ShouldBeNil)
			So(holding.Qty.Cmp(quantity), ShouldEqual, 0)
			So(holding.EntryPrice, ShouldNotBeNil)
			So(holding.EntryFee, ShouldNotBeNil)

			Convey("A Desk exit fills at the simulated bid and clears inventory", func() {
				So(wired.Desk.Sell(symbol, nil), ShouldBeNil)
				So(market.Paper.Drain(), ShouldBeNil)
				So(wired.Desk.OpenPositions(), ShouldEqual, 0)
				So(position.Status(), ShouldEqual, types.CLOSED)
			})
		})
	})
}
