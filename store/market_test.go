package store

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
)

func TestDecodeObservations(t *testing.T) {
	Convey("Given raw spot and futures frames in the general capture domain", t, func() {
		identity := hindsight.CaptureIdentity{Run: "run", Sequence: 1}
		spot := decodeObservations(identity, "trade", "wss://ws.kraken.com/v2", time.Unix(100, 0), []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","price":100,"qty":1,"side":"buy","timestamp":"2026-09-01T00:00:00Z"}]}`))
		future := decodeObservations(identity, "trade", "wss://futures.kraken.com/ws/v1", time.Unix(100, 0), []byte(`{"feed":"trade","product_id":"PF_XBTUSD","price":110,"qty":1,"side":"buy","time":1788220800000}`))
		So(spot, ShouldHaveLength, 1)
		So(future, ShouldHaveLength, 1)
		So(spot[0].Domain, ShouldEqual, "spot")
		So(future[0].Domain, ShouldEqual, "futures")
		So(future[0].Symbol, ShouldEqual, "PF_XBTUSD")
		So(future[0].TradePrice, ShouldEqual, 110)
		So(spot[0].TradePrice, ShouldEqual, 100)
		So(future[0].Capture, ShouldResemble, identity)
	})
}
