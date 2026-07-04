package trader

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOHLCMeasure(t *testing.T) {
	Convey("Given OHLC history capacity from config", t, func() {
		previousDepth := viper.GetInt("signals.feed_ring_capacity")
		viper.Set("signals.feed_ring_capacity", 8)
		defer viper.Set("signals.feed_ring_capacity", previousDepth)

		ohlc := NewOHLC()
		message := kraken.OHLCDataSlice{{
			Symbol:        "ALGO/USD",
			Open:          0.09875,
			High:          0.0988,
			Low:           0.09875,
			Close:         0.09875,
			Trades:        13,
			Volume:        16255.46368,
			Vwap:          0.09879,
			IntervalBegin: time.Date(2026, 7, 4, 11, 55, 0, 0, time.UTC),
			Interval:      5,
			Timestamp:     time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}}

		Convey("When OHLC data is measured", func() {
			at, err := ohlc.Measure(message)
			ring, ok := ohlc.history.cache.Load("ALGO/USD")

			Convey("It should store symbol history in the ClockRing", func() {
				So(err, ShouldBeNil)
				So(at.IsZero(), ShouldBeFalse)
				So(ok, ShouldBeTrue)
				So(ring, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkOHLCMeasure(b *testing.B) {
	previousDepth := viper.GetInt("signals.feed_ring_capacity")
	viper.Set("signals.feed_ring_capacity", 128)
	b.Cleanup(func() {
		viper.Set("signals.feed_ring_capacity", previousDepth)
	})

	ohlc := NewOHLC()
	message := kraken.OHLCDataSlice{{
		Symbol:        "ALGO/USD",
		Open:          0.09875,
		High:          0.0988,
		Low:           0.09875,
		Close:         0.09875,
		Trades:        13,
		Volume:        16255.46368,
		Vwap:          0.09879,
		IntervalBegin: time.Date(2026, 7, 4, 11, 55, 0, 0, time.UTC),
		Interval:      5,
		Timestamp:     time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	}}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := ohlc.Measure(message); err != nil {
			b.Fatal(err)
		}
	}
}
