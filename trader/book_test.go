package trader

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBookMeasure(t *testing.T) {
	Convey("Given book history capacity from config", t, func() {
		previousDepth := viper.GetInt("signals.feed_ring_capacity")
		viper.Set("signals.feed_ring_capacity", 8)
		defer viper.Set("signals.feed_ring_capacity", previousDepth)

		book := NewBook()
		message := kraken.BookDataSlice{{
			Symbol: "MATIC/USD",
			Bids: []kraken.BookLevel{{
				Price: 0.5666,
				Qty:   4831.75496356,
			}},
			Asks: []kraken.BookLevel{{
				Price: 0.5668,
				Qty:   4410.79769741,
			}},
			Checksum:  2439117997,
			Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}}

		Convey("When book data is measured", func() {
			at, err := book.Measure(message)
			ring, ok := book.history.cache.Load("MATIC/USD")

			Convey("It should store symbol history in the ClockRing", func() {
				So(err, ShouldBeNil)
				So(at.IsZero(), ShouldBeFalse)
				So(ok, ShouldBeTrue)
				So(ring, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkBookMeasure(b *testing.B) {
	previousDepth := viper.GetInt("signals.feed_ring_capacity")
	viper.Set("signals.feed_ring_capacity", 128)
	b.Cleanup(func() {
		viper.Set("signals.feed_ring_capacity", previousDepth)
	})

	book := NewBook()
	message := kraken.BookDataSlice{{
		Symbol: "MATIC/USD",
		Bids: []kraken.BookLevel{{
			Price: 0.5666,
			Qty:   4831.75496356,
		}},
		Asks: []kraken.BookLevel{{
			Price: 0.5668,
			Qty:   4410.79769741,
		}},
		Checksum:  2439117997,
		Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	}}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := book.Measure(message); err != nil {
			b.Fatal(err)
		}
	}
}
