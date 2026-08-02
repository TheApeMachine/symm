package tests

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMarketNewMarket(t *testing.T) {
	Convey("Given a list of symbols", t, func() {
		symbols := []*Symbol{
			NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("When NewMarket is initialized", func() {
			market := NewMarket(t.Context(), symbols)
			defer market.Close()

			So(market, ShouldNotBeNil)
			So(market.Public, ShouldNotBeNil)
			So(market.Private, ShouldNotBeNil)
			So(market.Level3, ShouldNotBeNil)
			So(market.State, ShouldEqual, Baseline)
		})
	})
}

func TestMarketTransition(t *testing.T) {
	Convey("Given a market in Baseline state", t, func() {
		symbols := []*Symbol{
			NewSymbol("SIM1/USD", 100.0, 42),
		}
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		Convey("When transitioning to FastPump", func() {
			market.Transition(FastPump)

			So(market.State, ShouldEqual, FastPump)
		})
	})
}

func TestMarketTick(t *testing.T) {
	Convey("Given a market", t, func() {
		symbols := []*Symbol{
			NewSymbol("SIM1/USD", 100.0, 42),
		}
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		Convey("When market.Tick is called", func() {
			market.Tick()

			So(market.Symbols[0].generator, ShouldNotBeNil)
		})
	})
}

func TestMarketWithMarket(t *testing.T) {
	Convey("Given WithMarket test wrapper", t, func() {
		symbols := []*Symbol{
			NewSymbol("SIM1/USD", 100.0, 42),
		}

		WithMarket(t, symbols, func(market *Market) {
			So(market, ShouldNotBeNil)
			market.Tick()
		})()
	})
}

func BenchmarkMarketTick(b *testing.B) {
	symbols := []*Symbol{
		NewSymbol("SIM1/USD", 100.0, 42),
		NewSymbol("SIM2/USD", 200.0, 43),
	}
	market := NewMarket(context.Background(), symbols)
	defer market.Close()

	for b.Loop() {
		market.Tick()
	}
}
