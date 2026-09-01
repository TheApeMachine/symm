package kraken

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

var level3FrameFixture = []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"MATIC/USD","bids":[{"order_id":"OEV5ES-SZKHN-DZJQHV","limit_price":0.5634,"order_qty":2400.5,"timestamp":"2023-10-06T18:20:25.097010033Z"},{"order_id":"OJM3GZ-LZQON-HVK7D2","limit_price":0.5633,"order_qty":1250.25,"timestamp":"2023-10-06T18:20:26.097010033Z"}],"asks":[{"order_id":"O2BN53-5RSB2-V3J57T","limit_price":0.5640,"order_qty":3500.7766862600,"timestamp":"2023-10-06T18:20:27.383408052Z"},{"order_id":"OWG5ZU-LHUHH-BICPEX","limit_price":0.5641,"order_qty":22149.62881248,"timestamp":"2023-10-06T18:20:50.842854530Z"}],"checksum":2841398499,"timestamp":"2023-10-06T18:21:00.097010033Z"}]}`)

func TestNewLevel3(t *testing.T) {
	Convey("Given a Level-3 frame with fixed-point order values", t, func() {
		level3 := NewLevel3(level3FrameFixture)

		Convey("The parsed book should retain values used by checksum construction", func() {
			So(level3.Data, ShouldHaveLength, 1)
			So(level3.Data[0].Bids, ShouldHaveLength, 2)
			So(level3.Data[0].Asks, ShouldHaveLength, 2)
			So(level3.Data[0].Asks[0].ChecksumLimitPrice(), ShouldEqual, "0.5640")
			So(level3.Data[0].Asks[0].ChecksumOrderQty(), ShouldEqual, "3500.7766862600")
			So(level3.Data[0].Asks[0].LimitPrice.String(), ShouldEqual, "0.5640")
			So(level3.Data[0].Asks[0].OrderQty.String(), ShouldEqual, "3500.7766862600")
		})
	})

	Convey("Given a Level-3 modification encoded in scientific notation", t, func() {
		level3 := NewLevel3([]byte(`{"channel":"level3","type":"update","data":[{"symbol":"AKE/USD","asks":[{"event":"modify","order_id":"order","limit_price":0.00567764,"order_qty":1e-05,"timestamp":"2026-08-13T11:41:06.617465962Z"}],"bids":[]}]}`))

		Convey("The minimum lot should retain its exact nonzero quantity", func() {
			So(level3.Data, ShouldHaveLength, 1)
			So(level3.Data[0].Asks, ShouldHaveLength, 1)
			So(level3.Data[0].Asks[0].OrderQty.String(), ShouldEqual, "0.00001")
			So(level3.Data[0].Asks[0].ChecksumOrderQty(), ShouldEqual, "0.00001")
		})
	})
}

func BenchmarkNewLevel3(b *testing.B) {
	for b.Loop() {
		NewLevel3(level3FrameFixture)
	}
}

/*
TestLevel3OrderResting pins the resting-liquidity predicate. Every Level-3
consumer that reads a price or a quantity off an order depends on it: a delete
reports liquidity being REMOVED, so counting it as displayed size reads
withdrawn size as resting size, and because a delete can be priced anywhere it
can also manufacture a crossed book out of a healthy one.
*/
func TestLevel3OrderResting(t *testing.T) {
	Convey("Given Level-3 orders", t, func() {
		price := decimal.NewFromFloat64(100)
		quantity := decimal.NewFromFloat64(5)

		Convey("An add is resting liquidity", func() {
			So(Level3Order{Event: "add", LimitPrice: price, OrderQty: quantity}.Resting(), ShouldBeTrue)
		})

		Convey("A modify still rests at its reported remaining quantity", func() {
			So(Level3Order{Event: "modify", LimitPrice: price, OrderQty: quantity}.Resting(), ShouldBeTrue)
		})

		Convey("An absent event is a snapshot order and rests", func() {
			So(Level3Order{LimitPrice: price, OrderQty: quantity}.Resting(), ShouldBeTrue)
		})

		Convey("A delete is not resting, whatever it is priced at", func() {
			So(Level3Order{Event: "delete", LimitPrice: price, OrderQty: quantity}.Resting(), ShouldBeFalse)
		})

		Convey("An order missing a price or quantity is not usable", func() {
			So(Level3Order{Event: "add", OrderQty: quantity}.Resting(), ShouldBeFalse)
			So(Level3Order{Event: "add", LimitPrice: price}.Resting(), ShouldBeFalse)
		})
	})
}
