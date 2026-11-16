package kraken

import (
	"strconv"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

const tickerREST = `{
	"error": [],
	"result": {
		"STRK/USD": {
			"a": ["2.10","1","1.000"],
			"b": ["2.09","1","1.000"],
			"c": ["2.10","100"],
			"v": ["1000","50000"],
			"p": ["2.00","2.00"],
			"t": [10, 200],
			"l": ["1.90","1.80"],
			"h": ["2.20","2.30"],
			"o": "1.75"
		}
	}
}`

func TestParseTicker(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When ParseTicker decodes a REST payload", func() {
			ticker := NewTicker()
			err := ticker.UnmarshalJSON([]byte(tickerREST))
			rows := ticker.Result

			Convey("row metrics match wire fields", func() {
				So(err, ShouldBeNil)
				So(rows, ShouldContainKey, "STRK/USD")

				row := rows["STRK/USD"]
				open, _ := strconv.ParseFloat("1.75", 64)
				last, _ := strconv.ParseFloat("2.10", 64)
				So(row.QuoteVolume24h(), ShouldEqual, 100_000)
				So(row.ChangePct(), ShouldEqual, (last-open)/open*100)
				So(row.LastTrade(), ShouldEqual, last)
				So(row.High24h(), ShouldEqual, 2.30)
				So(row.Low24h(), ShouldEqual, 1.80)
			})
		})

		Convey("When the venue returns EService:Unavailable", func() {
			ticker := NewTicker()
			err := ticker.UnmarshalJSON([]byte(`{"error":["EService:Unavailable"],"result":{}}`))
			rows := ticker.Result

			Convey("ParseTicker rejects the frame", func() {
				So(rows, ShouldBeNil)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "EService:Unavailable")
			})
		})
	})
}

func TestUnmarshalJSONTicker(t *testing.T) {
	Convey("Setup", t, func() {
		wire := NewTicker()

		Convey("When UnmarshalJSON loads REST rows", func() {
			So(wire.UnmarshalJSON([]byte(tickerREST)), ShouldBeNil)
			book := market.NewTicker(nil)
			wire.ToMarket(book)

			Convey("ToMarket projects volume and OHLC", func() {
				row := book.Of("STRK/USD")
				So(row, ShouldNotBeNil)
				So(row.VolumeUSD, ShouldEqual, 100_000)
				So(row.Last, ShouldEqual, 2.10)
				So(row.High24h, ShouldEqual, 2.30)
				So(row.Low24h, ShouldEqual, 1.80)
				So(row.Open, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func BenchmarkParseTicker(b *testing.B) {
	raw := []byte(tickerREST)

	b.ReportAllocs()

	for b.Loop() {
		ticker := NewTicker()
		_ = ticker.UnmarshalJSON(raw)
	}
}

func BenchmarkUnmarshalJSONTicker(b *testing.B) {
	wire := NewTicker()
	book := market.NewTicker(nil)
	raw := []byte(tickerREST)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = wire.UnmarshalJSON(raw)
		wire.ToMarket(book)
	}
}
