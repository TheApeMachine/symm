package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResolveBookTickSizePerSide(testingTB *testing.T) {
	Convey("Given bid and ask ladders with intra-side steps", testingTB, func() {
		tickSize, err := resolveBookTickSize(
			[]float64{100, 99.9, 99.8},
			[]float64{100.1, 100.2},
			0,
		)

		Convey("It should use the minimum intra-side increment", func() {
			So(err, ShouldBeNil)
			So(tickSize, ShouldAlmostEqual, 0.1, 1e-9)
		})
	})

	Convey("Given only touch prices on each side", testingTB, func() {
		Convey("When an instrument increment fallback is available", func() {
			tickSize, err := resolveBookTickSize(
				[]float64{50000},
				[]float64{50001},
				0.1,
			)

			So(err, ShouldBeNil)
			So(tickSize, ShouldAlmostEqual, 0.1, 1e-9)
		})
	})
}

// func TestBookElementToKrakenPreservesFeedType(testingTB *testing.T) {
// 	Convey("Given a buffered book element with feed_type update", testingTB, func() {
// 		element := []byte(`{
// 			"symbol":"BTC/EUR",
// 			"feed_type":"update",
// 			"timestamp":"2024-01-01T00:00:00Z",
// 			"bids":[{"price":"100","qty":"1"}],
// 			"asks":[{"price":"101","qty":"1"}]
// 		}`)

// 		update := bookElementToKraken("BTC/EUR", element, time.Unix(0, 0))

// 		Convey("It should preserve the feed type instead of forcing snapshot", func() {
// 			So(update.Type, ShouldEqual, "update")
// 			So(len(update.Bids), ShouldEqual, 1)
// 			So(len(update.Asks), ShouldEqual, 1)
// 		})
// 	})
// }

func TestSetInstrumentTickSize(testingTB *testing.T) {
	Convey("Given a symbol waiting for tick resolution", testingTB, func() {
		registry := NewSyncRegistry()
		registry.SetInstrumentTickSize("BTC/EUR", 0.1)

		state := registry.loadSymbol("BTC/EUR")

		Convey("It should store the exchange increment for later book configuration", func() {
			So(state.instrumentTickSize, ShouldAlmostEqual, 0.1, 1e-12)
		})
	})
}

func BenchmarkResolveBookTickSize(benchmark *testing.B) {
	bids := []float64{100, 99.9, 99.8, 99.7, 99.6}
	asks := []float64{100.1, 100.2, 100.3, 100.4, 100.5}

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for benchmark.Loop() {
		_, _ = resolveBookTickSize(bids, asks, 0)
	}
}
