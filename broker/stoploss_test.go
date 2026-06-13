package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
)

func TestStopLossRatchetAndEvaluate(t *testing.T) {
	Convey("Given a trailing stop on a long position", t, func() {
		stopLoss, err := NewStopLoss("BTC/USD", 1, 100, 50)

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

		Convey("It should trigger from executable bid when last price is stale", func() {
			bidTicker := &market.TickerUpdate{
				Symbol:    "BTC/USD",
				Last:      105,
				Bid:       104,
				Ask:       104.2,
				Timestamp: time.Now(),
			}

			ratcheted, ratchetErr := stopLoss.Ratchet(bidTicker)

			So(ratchetErr, ShouldBeNil)
			So(ratcheted, ShouldBeTrue)
			So(stopLoss.PeakPrice, ShouldEqual, 104)

			fallTicker := &market.TickerUpdate{
				Symbol:    "BTC/USD",
				Last:      105,
				Bid:       stopLoss.StopPrice - 0.01,
				Ask:       stopLoss.StopPrice + 0.02,
				Timestamp: time.Now(),
			}

			triggered, evaluateErr := stopLoss.Evaluate(fallTicker)

			So(evaluateErr, ShouldBeNil)
			So(triggered, ShouldBeTrue)
		})
	})
}

func TestDeriveTrailOffsetBehavior(t *testing.T) {
	Convey("Given spread and tape volatility", t, func() {
		Convey("It should widen offset when spread is elevated", func() {
			tight := DeriveTrailOffset(10, 0.001)
			wide := DeriveTrailOffset(100, 0.01)

			So(wide, ShouldBeGreaterThan, tight)
		})
	})
}

func TestStopLossDynamicVolatility(t *testing.T) {
	Convey("Given a stop loss with entry price of 100", t, func() {
		stopLoss, err := NewStopLoss("BTC/USD", 1, 100, 50)

		So(err, ShouldBeNil)
		initialOffset := stopLoss.Offset

		Convey("When a highly volatile sequence of prices is fed, the offset should widen", func() {
			prices := []float64{100, 105, 95, 110, 90, 115, 85}

			for _, price := range prices {
				ticker := &market.TickerUpdate{
					Symbol: "BTC/USD",
					Last:   price,
				}
				stopLoss.WidenOffsetFromTicker(ticker)
			}

			So(stopLoss.Offset, ShouldBeGreaterThan, initialOffset)
		})
	})
}

func BenchmarkDeriveTrailOffset(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		offset := DeriveTrailOffset(100, 0.01)
		_ = offset
	}
}

func BenchmarkStopLossEvaluate(benchmark *testing.B) {
	stopLoss, err := NewStopLoss("BTC/USD", 1, 100, 50)

	if err != nil {
		benchmark.Fatal(err)
	}

	ticker := &market.TickerUpdate{
		Symbol: "BTC/USD",
		Last:   99,
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = stopLoss.Evaluate(ticker)
	}
}

func BenchmarkStopLossRatchet(benchmark *testing.B) {
	stopLoss, err := NewStopLoss("BTC/USD", 1, 100, 50)

	if err != nil {
		benchmark.Fatal(err)
	}

	ticker := &market.TickerUpdate{
		Symbol: "BTC/USD",
		Last:   101,
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = stopLoss.Ratchet(ticker)
	}
}
