package market

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

const tickerFixture = `{
	"error": [],
	"result": {
		"XXBTZUSD": {
			"a": ["73269.00000", "2", "2.000"],
			"b": ["73268.90000", "1", "1.000"],
			"c": ["73272.50000", "0.00005100"],
			"v": ["1453.20135286", "1781.87420062"],
			"p": ["73446.35939", "73464.61394"],
			"t": [49477, 59122],
			"l": ["72362.80000", "72362.80000"],
			"h": ["74200.00000", "74200.00000"],
			"o": "73516.50000"
		}
	}
}`

func TestNewTickerInfo(t *testing.T) {
	Convey("Given a Kraken ticker payload", t, func() {
		info := TickerInfo{}

		Convey("It should unmarshal abbreviated ticker fields", func() {
			So(json.Unmarshal([]byte(tickerFixture), &public.Response{Result: &info}), ShouldBeNil)

			entry, ok := info["XXBTZUSD"]

			So(ok, ShouldBeTrue)
			So(entry.Open, ShouldEqual, "73516.50000")
			So(len(entry.Ask), ShouldEqual, 3)
		})
	})
}
