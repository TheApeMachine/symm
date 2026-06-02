package market

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBookLevelUnmarshalJSON(t *testing.T) {
	Convey("Given Kraken string-encoded book levels", t, func() {
		var level BookLevel

		err := sonic.Unmarshal([]byte(`{"price":"45283.5","qty":"0.10000000"}`), &level)

		Convey("It should parse price and qty and preserve raw strings", func() {
			So(err, ShouldBeNil)
			So(level.Price, ShouldEqual, 45283.5)
			So(level.Qty, ShouldEqual, 0.1)
			So(level.PriceRaw, ShouldEqual, "45283.5")
			So(level.QtyRaw, ShouldEqual, "0.10000000")
		})
	})
}
