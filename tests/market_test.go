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

			So(market, ShouldNotBeNil)
			So(market.Public, ShouldNotBeNil)
			So(market.Private, ShouldNotBeNil)
			So(market.Level3, ShouldNotBeNil)
			So(market.State, ShouldEqual, types.Baseline)
		})
	})
}

func TestMarketTransition(t *testing.T) {
	Convey("Given a market in Baseline state", t, func() {
		symbols := []*types.Symbol{
			types.NewSymbol("SIM1/USD", 100.0, 42),
		}
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		Convey("When transitioning to FastPump", func() {
			market.Transition(types.FastPump)

			So(market.State, ShouldEqual, types.FastPump)
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
