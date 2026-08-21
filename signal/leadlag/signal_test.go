package leadlag

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
)

var benchmarkTickerPrice float64

func TestTickerPrice(t *testing.T) {
	Convey("Given Kraken ticker price states", t, func() {
		Convey("A zero last represents no observation", func() {
			last := decimal.NewFromInt64(0)
			price, observed, err := tickerPrice(kraken.TickerData{Last: last})

			So(err, ShouldBeNil)
			So(observed, ShouldBeFalse)
			So(price, ShouldEqual, 0.0)
		})

		Convey("A positive last is admitted unchanged", func() {
			last := decimal.NewFromInt64(310)
			price, observed, err := tickerPrice(kraken.TickerData{Last: last})

			So(err, ShouldBeNil)
			So(observed, ShouldBeTrue)
			So(price, ShouldEqual, 310.0)
		})

		Convey("A missing last is an explicit error", func() {
			_, _, err := tickerPrice(kraken.TickerData{})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				"leadlag: ticker requires a last price")
		})
	})
}

func BenchmarkTickerPrice(b *testing.B) {
	last := decimal.NewFromInt64(310)
	ticker := kraken.TickerData{Last: last}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		benchmarkTickerPrice, _, _ = tickerPrice(ticker)
	}
}
