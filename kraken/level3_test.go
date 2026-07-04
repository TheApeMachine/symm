package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewLevel3DataSlice(t *testing.T) {
	Convey("Given Kraken level3 data payloads", t, func() {
		payload := []byte(`[{
			"symbol": "BTC/USD",
			"checksum": 291736120,
			"bids": [{
				"event": "add",
				"order_id": "OQCLML-BW3P3-BUCMWZ",
				"limit_price": 43125.3,
				"order_qty": 0.15,
				"timestamp": "2022-12-25T09:30:59.123456789Z"
			}],
			"asks": []
		}]`)

		levels := NewLevel3DataSlice(payload)

		Convey("It should decode order identity, price, and quantity", func() {
			So(len(levels), ShouldEqual, 1)

			level3 := levels[0]

			So(level3.Symbol, ShouldEqual, "BTC/USD")
			So(level3.Checksum, ShouldEqual, uint32(291736120))
			So(level3.Bids[0].OrderID, ShouldEqual, "OQCLML-BW3P3-BUCMWZ")
			So(level3.Bids[0].LimitPrice, ShouldAlmostEqual, 43125.3)
			So(level3.Bids[0].OrderQty, ShouldAlmostEqual, 0.15)
		})
	})
}
