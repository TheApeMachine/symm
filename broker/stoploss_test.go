package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

func TestStopLossRatchetAndEvaluate(t *testing.T) {
	Convey("Given a trailing stop on a long position", t, func() {
		stopLoss, err := NewStopLoss(
			"BTC/USD",
			1,
			100,
			0,
			config.ExitConfig{
				TrailDefault: 0.015,
				StopFloor:    0.012,
				SpreadScale:  0.5,
			},
		)

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
		exitConfig := config.ExitConfig{
			TrailDefault: 0.015,
			StopFloor:    0.012,
			SpreadScale:  0.5,
		}

		Convey("It should use the configured default trail", func() {
			offset := assessTrailOffset(exitConfig, 0)

			So(offset, ShouldEqual, 0.015)
		})

		Convey("It should widen offset when spread is elevated", func() {
			offset := assessTrailOffset(exitConfig, 100)

			So(offset, ShouldBeGreaterThan, 0.015)
		})
	})
}

func BenchmarkAssessTrailOffset(b *testing.B) {
	exitConfig := config.ExitConfig{
		TrailDefault: 0.015,
		StopFloor:    0.012,
		SpreadScale:  0.5,
	}

	b.ReportAllocs()

	for b.Loop() {
		offset := assessTrailOffset(exitConfig, 100)
		_ = offset
	}
}
