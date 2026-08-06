package tests

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests/types"
)

func TestMarketNewMarket(t *testing.T) {
	Convey("Given a list of symbols", t, func() {
		symbols := []*types.Symbol{
			types.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("When NewMarket is initialized", func() {
			market := NewMarket(t.Context(), symbols)
			defer market.Close()

			So(market != nil, ShouldBeTrue)
			So(market.Public != nil, ShouldBeTrue)
			So(market.Private != nil, ShouldBeTrue)
			So(market.Level3 != nil, ShouldBeTrue)
			So(market.State, ShouldEqual, types.Baseline)
		})
	})
}

func TestMarketTransition(t *testing.T) {
	Convey("Given a market in Baseline state", t, func() {
		symbols := []*types.Symbol{
			types.NewSymbol("SIM1/USD", 100.0, 42),
			types.NewSymbol("SIM2/USD", 100.0, 1337),
		}
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		Convey("When transitioning one symbol to FastPump", func() {
			market.Tick()
			peerBefore := market.latest["SIM2/USD"].Timestamp
			err := market.Transition("SIM1/USD", types.FastPump)

			So(err, ShouldBeNil)
			So(market.State, ShouldEqual, types.FastPump)
			So(market.generators["SIM1/USD"].IgnitionArmed(), ShouldBeTrue)
			So(market.generators["SIM2/USD"].IgnitionArmed(), ShouldBeFalse)
			So(market.latest["SIM2/USD"].Timestamp.After(peerBefore), ShouldBeTrue)
			liveBook := market.private.Book("SIM1/USD")
			So(liveBook, ShouldNotBeNil)
			So(liveBook.Bids.Levels, ShouldHaveLength, 1)
			So(liveBook.Asks.Levels, ShouldHaveLength, 1)

			pumped := market.generators["SIM1/USD"].Step()
			baseline := market.generators["SIM2/USD"].Step()

			So(pumped.ChangePct, ShouldBeGreaterThan, baseline.ChangePct)
		})

		Convey("When transitioning an unknown symbol", func() {
			err := market.Transition("UNKNOWN/USD", types.FastPump)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				`market: cannot transition unknown symbol "UNKNOWN/USD"`)
			So(market.State, ShouldEqual, types.Baseline)
		})
	})
}

func TestMarketWithMarket(t *testing.T) {
	Convey("Given WithMarket test wrapper", t, func() {
		symbols := []*types.Symbol{
			types.NewSymbol("SIM1/USD", 100.0, 42),
		}

		WithMarket(t, symbols, func(market *Market) {
			So(market, ShouldNotBeNil)
			market.Tick()
		})()
	})
}

func TestMarketWithAutoFill(t *testing.T) {
	symbols := []*types.Symbol{
		types.NewSymbol("SIM1/USD", 105.0, 42),
	}

	Convey(
		"Given an executable position lifecycle at the simulated venue",
		t, WithFixtureOrders(t, symbols, func(market *Market) {
			market.WithAutoFill()
			market.Tick()
			market.Tick()
			market.Tick()
			market.Tick()
		}))
}

func BenchmarkMarketTick(b *testing.B) {
	symbols := []*types.Symbol{
		types.NewSymbol("SIM1/USD", 100.0, 42),
		types.NewSymbol("SIM2/USD", 200.0, 43),
	}
	market := NewMarket(context.Background(), symbols)
	defer market.Close()

	for b.Loop() {
		market.Tick()
	}
}
