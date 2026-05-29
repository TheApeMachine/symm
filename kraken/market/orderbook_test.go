package market

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

const depthFixture = `{
	"error": [],
	"result": {
		"XXBTZUSD": {
			"asks": [["73262.80000", "1.120", 1780081237]],
			"bids": [["73262.70000", "0.075", 1780081237]]
		}
	}
}`

func TestNewOrderBook(t *testing.T) {
	Convey("Given a Kraken depth payload", t, func() {
		book := OrderBook{}

		Convey("It should unmarshal bid and ask rows on the wire", func() {
			So(json.Unmarshal([]byte(depthFixture), &public.Response{Result: &book}), ShouldBeNil)

			side, ok := book["XXBTZUSD"]

			So(ok, ShouldBeTrue)
			So(len(side.Asks), ShouldEqual, 1)
			So(side.Asks[0][0], ShouldEqual, "73262.80000")
			So(side.Bids[0][1], ShouldEqual, "0.075")
		})
	})
}
