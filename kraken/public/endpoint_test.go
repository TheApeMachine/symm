package public

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEndpointSignPath(t *testing.T) {
	Convey("Given Kraken endpoint constants", t, func() {
		Convey("It should strip the API host for signing", func() {
			So(EndpointTypeAssetPairs.SignPath(), ShouldEqual, "/0/public/AssetPairs")
			So(EndpointAddOrder.SignPath(), ShouldEqual, "/0/private/AddOrder")
		})

		Convey("It should expose channel name constants", func() {
			So(TickerChannel, ShouldEqual, "ticker")
			So(BookChannel, ShouldEqual, "book")
			So(ExecutionsChannel, ShouldEqual, "executions")
		})
	})
}
