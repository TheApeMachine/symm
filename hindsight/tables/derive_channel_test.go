package tables

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestDeriveChannelFuturesFamilies(t *testing.T) {
	Convey("futures tape channels route to futures Iceberg families", t, func() {
		ticker := data.NewMeasurement[float64]("websocket", nil)
		ticker.SetProvenance("channel", "futures_ticker")
		So(deriveChannel(ticker), ShouldEqual, "futures_ticker")

		trade := data.NewMeasurement[float64]("websocket", nil)
		trade.SetMetadata("type", "futures_trade")
		So(deriveChannel(trade), ShouldEqual, "futures_trade")
	})
}
