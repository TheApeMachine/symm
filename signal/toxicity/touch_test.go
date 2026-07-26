package toxicity

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

/*
TestAttributeTouchSide proves fill attribution requires exact executable price
identity after the full-market proof has established the surrounding flow.
*/
func TestAttributeTouchSide(t *testing.T) {
	bid, err := decimal.NewFromString("100.00")

	if err != nil {
		t.Fatal(err)
	}

	ask, err := decimal.NewFromString("100.02")

	if err != nil {
		t.Fatal(err)
	}

	Convey("Given an executable bid and ask on Kraken's tick lattice", t, func() {
		Convey("A trade at the ask should consume the ask", func() {
			side, err := attributeTouchSide(*ask, bid, ask)
			So(err, ShouldBeNil)
			So(side, ShouldEqual, touchSideAsk)
		})

		Convey("A trade at the bid should consume the bid", func() {
			side, err := attributeTouchSide(*bid, bid, ask)
			So(err, ShouldBeNil)
			So(side, ShouldEqual, touchSideBid)
		})

		Convey("A trade one tick beyond the ask should not be a touch fill", func() {
			outside, err := decimal.NewFromString("100.03")
			So(err, ShouldBeNil)
			side, err := attributeTouchSide(*outside, bid, ask)
			So(err, ShouldBeNil)
			So(side, ShouldEqual, touchSideNone)
		})
	})
}
