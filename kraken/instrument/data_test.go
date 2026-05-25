package instrument

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestDataPair(t *testing.T) {
	convey.Convey("Given instrument channel data", t, func() {
		row := Data{
			Symbol:  "BTC/EUR",
			Base:    "BTC",
			Quote:   "EUR",
			Status:  "online",
			CostMin: 0.45,
		}

		convey.Convey("It should map into asset.Pair", func() {
			pair := row.Pair()
			convey.So(pair.Wsname, convey.ShouldEqual, "BTC/EUR")
			convey.So(pair.Base, convey.ShouldEqual, "BTC")
			convey.So(pair.Quote, convey.ShouldEqual, "EUR")
			convey.So(pair.Costmin, convey.ShouldEqual, "0.45")
			convey.So(pair.Status, convey.ShouldEqual, "online")
		})
	})
}
