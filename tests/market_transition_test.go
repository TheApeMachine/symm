package tests

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	tes "github.com/theapemachine/symm/tests/types"
)

func TestMarketTransition(t *testing.T) {
	Convey("Given two independently generated symbols", t, func() {
		market := NewMarket(t.Context(), []*tes.Symbol{
			tes.NewSymbol("SIM1/USD", 100, 42),
			tes.NewSymbol("SIM2/USD", 100, 1337),
		})
		defer market.Close()
		market.Tick()
		peerBefore := market.latest["SIM2/USD"].Timestamp

		Convey("A known state should advance the shared timeline", func() {
			err := market.Transition("SIM1/USD", tes.FastPump)
			So(err, ShouldBeNil)
			So(market.State, ShouldEqual, tes.FastPump)
			So(market.generators["SIM1/USD"].IgnitionArmed(), ShouldBeTrue)
			So(market.generators["SIM2/USD"].IgnitionArmed(), ShouldBeFalse)
			So(market.latest["SIM2/USD"].Timestamp.After(peerBefore), ShouldBeTrue)
		})

		Convey("An unknown symbol should be rejected", func() {
			err := market.Transition("UNKNOWN/USD", tes.FastPump)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				`market: cannot transition unknown symbol "UNKNOWN/USD"`)
		})

		Convey("An unknown state should be rejected", func() {
			err := market.Transition("SIM1/USD", tes.MarketState(10_000))
			So(err, ShouldNotBeNil)
		})
	})
}
