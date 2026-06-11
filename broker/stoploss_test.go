package broker

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestStopLossRatchetAndEvaluate(t *testing.T) {
	Convey("Given a trailing stop on a long position", t, func() {
		stopLoss, err := NewStopLoss("BTC/USD", 1, 100, "5.0.0.0.0", 0)

		So(err, ShouldBeNil)

		ticker := &market.TickerUpdate{
			Symbol:    "BTC/USD",
			Last:      105,
			Timestamp: time.Now(),
		}

		Convey("It should not trigger before price falls through the stop", func() {
			ratcheted, ratchetErr := stopLoss.Ratchet(ticker)

			So(ratchetErr, ShouldBeNil)
			So(ratcheted, ShouldBeTrue)
			So(stopLoss.PeakPrice, ShouldEqual, 105)

			triggered, evaluateErr := stopLoss.Evaluate(ticker)

			So(evaluateErr, ShouldBeNil)
			So(triggered, ShouldBeFalse)
		})

		Convey("It should trigger when price falls through the ratcheted stop", func() {
			_, ratchetErr := stopLoss.Ratchet(ticker)

			So(ratchetErr, ShouldBeNil)

			fallTicker := &market.TickerUpdate{
				Symbol:    "BTC/USD",
				Last:      stopLoss.StopPrice - 0.01,
				Timestamp: time.Now(),
			}

			triggered, evaluateErr := stopLoss.Evaluate(fallTicker)

			So(evaluateErr, ShouldBeNil)
			So(triggered, ShouldBeTrue)
		})
	})
}

func TestAssessTrailOffset(t *testing.T) {
	Convey("Given exit trail config", t, func() {
		viper.Set("trading.exit.stop_floor", 0.012)
		viper.Set("trading.exit.trail_tight", 0.01)
		viper.Set("trading.exit.trail_wide", 0.03)
		viper.Set("trading.exit.trail_revert", 0.015)
		viper.Set("trading.exit.trail_default", 0.015)
		viper.Set("trading.exit.spread_scale", 0.5)

		Convey("It should pick tighter trails for ignition entries", func() {
			offset := assessTrailOffset("5.0.0.0.0", 0)

			So(offset, ShouldEqual, 0.012)
		})

		Convey("It should pick wider trails for organic trend entries", func() {
			offset := assessTrailOffset("7.0", 0)

			So(offset, ShouldEqual, 0.03)
		})

		Convey("It should widen offset when spread is elevated", func() {
			offset := assessTrailOffset("8.0", 100)

			So(offset, ShouldBeGreaterThan, 0.015)
		})
	})
}
