package tests

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMarketNewMarket(t *testing.T) {
	Convey("Given one valid symbol", t, func() {
		market := NewMarket(t.Context(), []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100, 42),
		})
		defer market.Close()

		Convey("The complete fixture venue should initialize", func() {
			So(market.Public != nil, ShouldBeTrue)
			So(market.Private != nil, ShouldBeTrue)
			So(market.Level3 != nil, ShouldBeTrue)
			So(market.State, ShouldEqual, testtypes.Baseline)
		})
	})
}

func BenchmarkMarketTick(b *testing.B) {
	market := NewMarket(context.Background(), []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100, 42),
		testtypes.NewSymbol("SIM2/USD", 200, 43),
	})
	defer market.Close()

	for b.Loop() {
		market.Tick()
	}
}
