package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTickerUpdateResolveValue(t *testing.T) {
	Convey("Given a ticker with change_pct", t, func() {
		ticker := &TickerUpdate{
			Symbol:    "BTC/EUR",
			Last:      50000,
			ChangePct: 0.02,
		}

		Convey("It should return change_pct", func() {
			value, err := ticker.ResolveValue()

			So(err, ShouldBeNil)
			So(value, ShouldEqual, 0.02)
		})
	})

	Convey("Given a book-triggered ticker without 24h summary", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Bid:    49990,
			Ask:    50010,
		}

		Convey("It should derive value from touch spread over mid", func() {
			value, err := ticker.ResolveValue()

			So(err, ShouldBeNil)
			So(value, ShouldAlmostEqual, 20.0/50000.0, 0.0001)
		})
	})

	Convey("Given a ticker with absolute change but no change_pct", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   50000,
			Change: 250,
		}

		Convey("It should derive relative value from change over price", func() {
			value, err := ticker.ResolveValue()

			So(err, ShouldBeNil)
			So(value, ShouldEqual, 0.005)
		})
	})
}

func TestTickerUpdateResolveVolume(t *testing.T) {
	Convey("Given a ticker with 24h volume", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   50000,
			Volume: 12.5,
		}

		Convey("It should return session volume", func() {
			volume, err := ticker.ResolveVolume(50000)

			So(err, ShouldBeNil)
			So(volume, ShouldEqual, 12.5)
		})
	})

	Convey("Given a ticker with touch quantities only", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   50000,
			AskQty: 0.4,
			BidQty: 0.6,
		}

		Convey("It should return combined touch depth", func() {
			volume, err := ticker.ResolveVolume(50000)

			So(err, ShouldBeNil)
			So(volume, ShouldEqual, 1)
		})
	})

	Convey("Given a book-triggered ticker without sizes", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Bid:    49990,
			Ask:    50010,
		}

		Convey("It should derive volume from touch spread over price", func() {
			volume, err := ticker.ResolveVolume(50000)

			So(err, ShouldBeNil)
			So(volume, ShouldAlmostEqual, 20.0/50000.0, 0.0001)
		})
	})
	Convey("Given a snapshot ticker with last and change_pct but no session volume", t, func() {
		ticker := &TickerUpdate{
			Symbol:    "BTC/EUR",
			Last:      50000,
			ChangePct: 0.02,
		}

		Convey("It should derive notional volume from value over price", func() {
			volume, err := ticker.ResolveVolume(50000)

			So(err, ShouldBeNil)
			So(volume, ShouldEqual, 1000)
		})
	})

	Convey("Given a ticker with session high and low but no volume", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   50000,
			High:   51000,
			Low:    49000,
		}

		Convey("It should derive volume from the session range", func() {
			volume, err := ticker.ResolveVolume(50000)

			So(err, ShouldBeNil)
			So(volume, ShouldEqual, 2000)
		})
	})
}

func TestTickerUpdateCompleteSymbol(t *testing.T) {
	Convey("Given a book-triggered ticker", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Bid:    49990,
			Ask:    50010,
			AskQty: 1,
			BidQty: 1,
		}
		at := time.Unix(100, 0)

		Convey("It should build a validated symbol row", func() {
			row, err := ticker.CompleteSymbol(1, at)

			So(err, ShouldBeNil)
			So(row, ShouldNotBeNil)
			So(row.Validate(), ShouldBeNil)
			So(row.Value, ShouldAlmostEqual, 20.0/50000.0, 0.0001)
		})
	})

	Convey("Given a book-triggered ticker without touch quantities", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Bid:    49990,
			Ask:    50010,
		}
		at := time.Unix(100, 0)

		Convey("It should still build a validated symbol row", func() {
			row, err := ticker.CompleteSymbol(1, at)

			So(err, ShouldBeNil)
			So(row, ShouldNotBeNil)
			So(row.Validate(), ShouldBeNil)
			So(row.Volume, ShouldAlmostEqual, 20.0/50000.0, 0.0001)
		})
	})
}

func TestTickerUpdateResolveValueHighLow(t *testing.T) {
	Convey("Given a ticker with session high and low", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   50000,
			High:   51000,
			Low:    49000,
		}

		Convey("It should derive value from the session range", func() {
			value, err := ticker.ResolveValue()

			So(err, ShouldBeNil)
			So(value, ShouldEqual, 2000.0/50000.0)
		})
	})
}

func BenchmarkTickerUpdateResolveValue(b *testing.B) {
	ticker := &TickerUpdate{
		Symbol: "BTC/EUR",
		Bid:    49990,
		Ask:    50010,
	}

	for b.Loop() {
		_, _ = ticker.ResolveValue()
	}
}
