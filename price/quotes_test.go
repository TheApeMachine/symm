package price

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
)

func TestParseEventTime(t *testing.T) {
	Convey("Given Kraken timestamp strings", t, func() {
		Convey("It should parse RFC3339Nano", func() {
			parsed := ParseEventTime("2026-01-01T12:00:00.123456789Z")
			So(parsed.IsZero(), ShouldBeFalse)
			So(parsed.UTC().Format(time.RFC3339), ShouldEqual, "2026-01-01T12:00:00Z")
		})

		Convey("It should return zero time for empty or malformed input", func() {
			So(ParseEventTime(""), ShouldEqual, time.Time{})
			So(ParseEventTime("not-a-time").IsZero(), ShouldBeTrue)
		})
	})
}

func TestPredictionLastQuoteAndVolatility(t *testing.T) {
	Convey("Given ticker observations", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		prediction := NewPrediction(ctx, pool)
		at := "2026-01-01T12:00:00Z"

		prediction.observeTicker(market.TickerRow{Symbol: "BTC/EUR", Last: 100, Bid: 99.9, Ask: 100.1, Timestamp: at})
		prediction.observeTicker(market.TickerRow{Symbol: "BTC/EUR", Last: 101, Bid: 100.9, Ask: 101.1, Timestamp: at})

		last, bid, ask, observedAt, ok := prediction.LastQuote("BTC/EUR")
		volatility := prediction.RecentVolatility("BTC/EUR")

		Convey("It should cache quote fields and track recent volatility", func() {
			So(ok, ShouldBeTrue)
			So(last, ShouldAlmostEqual, 101, 1e-12)
			So(bid, ShouldAlmostEqual, 100.9, 1e-12)
			So(ask, ShouldAlmostEqual, 101.1, 1e-12)
			So(observedAt.IsZero(), ShouldBeFalse)
			So(volatility, ShouldBeGreaterThan, 0)
			So(prediction.LastPrice("BTC/EUR"), ShouldAlmostEqual, 101, 1e-12)
		})
	})
}

func BenchmarkParseEventTime(b *testing.B) {
	value := "2026-01-01T12:00:00.123456789Z"

	for b.Loop() {
		_ = ParseEventTime(value)
	}
}
