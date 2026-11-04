package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseTickerQuoteVolume(t *testing.T) {
	Convey("ticker row yields 24h quote notional and open-to-last change", t, func() {
		raw := []byte(`{
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
		}`)

		rows, err := ParseTicker(raw)
		So(err, ShouldBeNil)
		So(rows, ShouldContainKey, "STRK/USD")

		row := rows["STRK/USD"]
		So(row.QuoteVolume24h(), ShouldEqual, 100_000)
		So(row.ChangePct(), ShouldAlmostEqual, (2.10-1.75)/1.75*100, 1e-9)
		So(row.LastTrade(), ShouldEqual, 2.10)
		So(row.High24h(), ShouldEqual, 2.30)
		So(row.Low24h(), ShouldEqual, 1.80)
	})
}
