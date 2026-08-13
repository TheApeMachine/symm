package types

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestQuoteHistoryObserve(t *testing.T) {
	Convey("Given out-of-order quotes inside a bounded history", t, func() {
		history := NewQuoteHistory(3)
		base := time.Unix(1_700_008_000, 0).UTC()
		So(history.Observe(quoteTicker(102, base.Add(2*time.Second))), ShouldBeTrue)
		So(history.Observe(quoteTicker(100, base)), ShouldBeTrue)
		So(history.Observe(quoteTicker(101, base.Add(time.Second))), ShouldBeTrue)

		Convey("It should select the newest quote no later than the event", func() {
			quote, found := history.At("BTC/USD", base.Add(1500*time.Millisecond))
			So(found, ShouldBeTrue)
			So(quote.Bid.Float64(), ShouldEqual, 100.99)
			So(quote.Ask.Float64(), ShouldEqual, 101.01)
		})

		Convey("It should never explain an older event with a later quote", func() {
			_, found := history.At("BTC/USD", base.Add(-time.Nanosecond))
			So(found, ShouldBeFalse)
		})
	})
}

func quoteTicker(midpoint float64, at time.Time) kraken.TickerData {
	return kraken.TickerData{
		Symbol: "BTC/USD",
		Bid: decimal.NewFromFloat64(midpoint - 0.01),
		Ask: decimal.NewFromFloat64(midpoint + 0.01),
		Timestamp: at,
	}
}

func BenchmarkQuoteHistoryObserve(b *testing.B) {
	history := NewQuoteHistory(128)
	base := time.Unix(1_700_008_000, 0).UTC()
	b.ReportAllocs()

	for b.Loop() {
		history.Observe(quoteTicker(100, base))
		_, _ = history.At("BTC/USD", base)
	}
}
