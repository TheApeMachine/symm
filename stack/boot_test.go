package stack_test

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestBooter_Test proves the injected paper Conn closes the real production
Desk, Position, execution, and Balance loop against the simulated market.
*/
func TestBooter_Test(t *testing.T) {
	Convey("Given ambient configuration that conflicts with the test market", t, func() {
		previousBuffer := viper.Get("system.websocket.channel.buffer")
		previousBars := viper.Get("signals.volume_clock_bars_per_day")
		viper.Set("system.websocket.channel.buffer", 1)
		viper.Set("signals.volume_clock_bars_per_day", 37)
		market := tests.NewMarket(t.Context(), 1)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			if wired != nil {
				So(wired.Close(), ShouldBeNil)
			}

			market.Close()
			viper.Set("system.websocket.channel.buffer", previousBuffer)
			viper.Set("signals.volume_clock_bars_per_day", previousBars)
		})

		Convey("The graph should use only its deterministic test configuration", func() {
			So(viper.GetInt("system.websocket.channel.buffer"), ShouldEqual, 4096)
			So(cap(wired.Channel), ShouldEqual, 4096)
			So(viper.GetFloat64("signals.volume_clock_bars_per_day"), ShouldEqual, 0.0)
			So(wired.Close(), ShouldBeNil)
			wired = nil
			So(viper.GetInt("system.websocket.channel.buffer"), ShouldEqual, 1)
			So(viper.GetFloat64("signals.volume_clock_bars_per_day"), ShouldEqual, 37.0)
		})
	})

	Convey("Given the complete production graph on a simulated market", t, func() {
		market := tests.NewMarket(t.Context(), 1)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})
		So(market.Warmup(wired.Crypto.Step), ShouldBeNil)
		symbol := market.Symbols[0]
		quantity := decimal.NewFromInt64(1)

		Convey("A Desk entry fills at the simulated ask and updates inventory", func() {
			position, err := wired.Desk.Buy(
				types.NewHolding(t.Context(), symbol, quantity),
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
				So(wired.Desk.Sell(symbol), ShouldBeNil)
				So(market.Paper.Drain(), ShouldBeNil)
				So(wired.Desk.OpenPositions(), ShouldEqual, 0)
				So(position.Status(), ShouldEqual, types.CLOSED)
			})
		})
	})
}
