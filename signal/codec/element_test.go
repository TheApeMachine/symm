package codec

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPeekElementOK(testingTB *testing.T) {
	Convey("Given a book element", testingTB, func() {
		element := []byte(`{
			"symbol":"BTC/USD",
			"side":"buy",
			"price":50000,
			"qty":0.5,
			"bids":[{"price":49990,"qty":1.25}],
			"asks":[{"price":50010,"qty":2.5}],
			"timestamp":"2026-06-17T12:00:00Z"
		}`)

		Convey("When nested paths are read", func() {
			price, priceOK := PeekElementOK[float64](element, "price")
			side, sideOK := PeekElementOK[string](element, "side")
			bidQty, bidQtyOK := PeekElementOK[float64](element, "bids.0.qty")

			Convey("It should return typed values", func() {
				So(priceOK, ShouldBeTrue)
				So(price, ShouldEqual, 50000)
				So(sideOK, ShouldBeTrue)
				So(side, ShouldEqual, "buy")
				So(bidQtyOK, ShouldBeTrue)
				So(bidQty, ShouldEqual, 1.25)
			})
		})
	})
}

func TestElementTime(testingTB *testing.T) {
	Convey("Given an element timestamp", testingTB, func() {
		element := []byte(`{"timestamp":"2026-06-17T12:00:00Z"}`)
		eventAt, eventOK := ElementTime(element, "timestamp")

		Convey("It should parse RFC3339 timestamps", func() {
			So(eventOK, ShouldBeTrue)
			So(eventAt, ShouldEqual, time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
		})
	})
}

func TestEachBookLevelElement(testingTB *testing.T) {
	Convey("Given a book element", testingTB, func() {
		element := []byte(`{
			"bids":[{"price":100,"qty":1},{"price":99,"qty":2}],
			"asks":[{"price":101,"qty":3}]
		}`)

		bidCount := 0

		EachBookLevelElement(element, "bids", func(price float64, qty float64) {
			bidCount++

			if bidCount == 1 {
				So(price, ShouldEqual, 100)
				So(qty, ShouldEqual, 1)
			}
		})

		Convey("It should visit every level", func() {
			So(bidCount, ShouldEqual, 2)
		})
	})
}

func TestTouchSpread(testingTB *testing.T) {
	Convey("Given a price series", testingTB, func() {
		spread, spreadOK := TouchSpread([]float64{100, 101.5, 99.5})

		Convey("It should return the observed range", func() {
			So(spreadOK, ShouldBeTrue)
			So(spread, ShouldEqual, 2)
		})
	})
}

func BenchmarkPeekElementOK(benchmark *testing.B) {
	element := []byte(`{"price":50000,"bids":[{"price":49990,"qty":1.25}]}`)

	for benchmark.Loop() {
		_, _ = PeekElementOK[float64](element, "bids.0.qty")
	}
}
