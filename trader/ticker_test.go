package trader

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTickerMeasure(t *testing.T) {
	Convey("Given ticker history capacity from config", t, func() {
		previousDepth := viper.GetInt("signals.feed_ring_capacity")
		viper.Set("signals.feed_ring_capacity", 8)
		defer viper.Set("signals.feed_ring_capacity", previousDepth)

		ticker := NewTicker()
		message := kraken.TickerDataSlice{{
			Symbol:    "BTC/USD",
			Bid:       99,
			Ask:       101,
			Last:      100,
			Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}}

		Convey("When ticker data is measured", func() {
			at, err := ticker.Measure(message)
			ring, ok := ticker.history.cache.Load("BTC/USD")

			Convey("It should store symbol history in the ClockRing", func() {
				So(err, ShouldBeNil)
				So(at.IsZero(), ShouldBeFalse)
				So(ok, ShouldBeTrue)
				So(ring, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkTickerMeasure(b *testing.B) {
	previousDepth := viper.GetInt("signals.feed_ring_capacity")
	viper.Set("signals.feed_ring_capacity", 128)
	b.Cleanup(func() {
		viper.Set("signals.feed_ring_capacity", previousDepth)
	})

	ticker := NewTicker()
	message := kraken.TickerDataSlice{{
		Symbol:    "BTC/USD",
		Bid:       99,
		Ask:       101,
		Last:      100,
		Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	}}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := ticker.Measure(message); err != nil {
			b.Fatal(err)
		}
	}
}
