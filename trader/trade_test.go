package trader

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTradeMeasure(t *testing.T) {
	Convey("Given trade history capacity from config", t, func() {
		previousDepth := viper.GetInt("signals.feed_ring_capacity")
		viper.Set("signals.feed_ring_capacity", 8)
		defer viper.Set("signals.feed_ring_capacity", previousDepth)

		trade := NewTrade()
		message := kraken.TradeDataSlice{{
			Symbol:    "MATIC/USD",
			Side:      "buy",
			Price:     0.5147,
			Qty:       6423.46326,
			OrderType: "limit",
			TradeID:   4665846,
			Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}}

		Convey("When trade data is measured", func() {
			at, err := trade.Measure(message)
			ring, ok := trade.history.cache.Load("MATIC/USD")

			Convey("It should store symbol history in the ClockRing", func() {
				So(err, ShouldBeNil)
				So(at.IsZero(), ShouldBeFalse)
				So(ok, ShouldBeTrue)
				So(ring, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkTradeMeasure(b *testing.B) {
	previousDepth := viper.GetInt("signals.feed_ring_capacity")
	viper.Set("signals.feed_ring_capacity", 128)
	b.Cleanup(func() {
		viper.Set("signals.feed_ring_capacity", previousDepth)
	})

	trade := NewTrade()
	message := kraken.TradeDataSlice{{
		Symbol:    "MATIC/USD",
		Side:      "buy",
		Price:     0.5147,
		Qty:       6423.46326,
		OrderType: "limit",
		TradeID:   4665846,
		Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	}}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := trade.Measure(message); err != nil {
			b.Fatal(err)
		}
	}
}
