package trader

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLevel3Measure(t *testing.T) {
	Convey("Given level3 history capacity from config", t, func() {
		previousDepth := viper.GetInt("signals.feed_ring_capacity")
		viper.Set("signals.feed_ring_capacity", 8)
		defer viper.Set("signals.feed_ring_capacity", previousDepth)

		level3 := NewLevel3()
		message := kraken.Level3DataSlice{{
			Symbol:    "BTC/USD",
			Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
			Checksum:  291736120,
			Bids: []kraken.Level3Order{{
				Event:      "add",
				OrderID:    "OQCLML-BW3P3-BUCMWZ",
				LimitPrice: 43125.3,
				OrderQty:   0.15,
				Timestamp:  time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
			}},
		}}

		Convey("When level3 data is measured", func() {
			at, err := level3.Measure(message)
			ring, ok := level3.history.cache.Load("BTC/USD")

			Convey("It should store symbol history in the ClockRing", func() {
				So(err, ShouldBeNil)
				So(at.IsZero(), ShouldBeFalse)
				So(ok, ShouldBeTrue)
				So(ring, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkLevel3Measure(b *testing.B) {
	previousDepth := viper.GetInt("signals.feed_ring_capacity")
	viper.Set("signals.feed_ring_capacity", 128)
	b.Cleanup(func() {
		viper.Set("signals.feed_ring_capacity", previousDepth)
	})

	level3 := NewLevel3()
	message := kraken.Level3DataSlice{{
		Symbol:    "BTC/USD",
		Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		Checksum:  291736120,
		Bids: []kraken.Level3Order{{
			Event:      "add",
			OrderID:    "OQCLML-BW3P3-BUCMWZ",
			LimitPrice: 43125.3,
			OrderQty:   0.15,
			Timestamp:  time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}},
	}}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := level3.Measure(message); err != nil {
			b.Fatal(err)
		}
	}
}
