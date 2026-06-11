package signal

import (
	"container/ring"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestRingMarketRow(t *testing.T) {
	Convey("Given two trades in the ring", t, func() {
		measurements := ring.New(4)
		at := time.Unix(100, 0)

		for index, price := range []float64{100, 101} {
			measurements.Value = &krakenmarket.TradeUpdate{
				Symbol:    "BTC/USD",
				Price:     price,
				Qty:       1,
				Timestamp: at.Add(time.Duration(index) * time.Second),
			}
			measurements = measurements.Next()
		}

		row, elapsed, volume, spread, err := RingMarketRow("BTC/USD", measurements, at.Add(2*time.Second))

		Convey("It should build a validated market row", func() {
			So(err, ShouldBeNil)
			So(row, ShouldNotBeNil)
			So(row.Validate(), ShouldBeNil)
			So(elapsed, ShouldBeGreaterThan, 0)
			So(volume, ShouldBeGreaterThan, 0)
			So(spread, ShouldBeGreaterThan, 0)
		})
	})
}

func TestRingQuote(t *testing.T) {
	Convey("Given a single book touch in the ring", t, func() {
		measurements := ring.New(4)
		at := time.Unix(100, 0)
		book := &krakenmarket.BookUpdate{
			Symbol:    "BTC/USD",
			Timestamp: at.Add(-time.Second),
			Bids:      []krakenmarket.BookLevel{{Price: 99, Qty: 1}},
			Asks:      []krakenmarket.BookLevel{{Price: 101, Qty: 1}},
		}

		measurements.Value = book
		measurements = measurements.Next()

		row, elapsed, volume, spread, err := RingQuote("BTC/USD", measurements, at)

		Convey("It should fall back to the touch quote", func() {
			So(err, ShouldBeNil)
			So(row, ShouldNotBeNil)
			So(row.Validate(), ShouldBeNil)
			So(elapsed, ShouldBeGreaterThan, 0)
			So(volume, ShouldBeGreaterThan, 0)
			So(spread, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkRingQuote(b *testing.B) {
	measurements := ring.New(4)
	at := time.Unix(100, 0)
	book := &krakenmarket.BookUpdate{
		Symbol:    "BTC/USD",
		Timestamp: at.Add(-time.Second),
		Bids:      []krakenmarket.BookLevel{{Price: 99, Qty: 1}},
		Asks:      []krakenmarket.BookLevel{{Price: 101, Qty: 1}},
	}

	measurements.Value = book
	measurements = measurements.Next()

	for b.Loop() {
		_, _, _, _, _ = RingQuote("BTC/USD", measurements, at)
	}
}
