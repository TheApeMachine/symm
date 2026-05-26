package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseTickerChangePctFraction(t *testing.T) {
	Convey("Given Kraken percentage-point change_pct", t, func() {
		payload := []byte(`{"symbol":"BTC/EUR","last":100,"change_pct":5.25}`)

		Convey("It should store fractional change", func() {
			row, ok := parseTickerEntry(payload)

			So(ok, ShouldBeTrue)
			So(row.ChangePct, ShouldAlmostEqual, 0.0525, 0.0001)
		})
	})
}
