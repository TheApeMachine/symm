package manifold

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBookReady(t *testing.T) {
	Convey("Given intensity leaders without L3 books", t, func() {
		candidates := []intensityCandidate{
			{symbol: "HOT/USD", intensity: 10},
			{symbol: "BTC/USD", intensity: 5},
		}
		source := newTestBookSource("BTC/USD")

		Convey("It should drop bookless symbols", func() {
			ready := bookReady(candidates, source)
			So(ready, ShouldHaveLength, 1)
			So(ready[0].symbol, ShouldEqual, "BTC/USD")
		})
	})
}

func BenchmarkBookReady(b *testing.B) {
	candidates := []intensityCandidate{
		{symbol: "HOT/USD", intensity: 10},
		{symbol: "BTC/USD", intensity: 5},
		{symbol: "ETH/USD", intensity: 3},
	}
	source := newTestBookSource("BTC/USD", "ETH/USD")

	b.ReportAllocs()

	for b.Loop() {
		_ = bookReady(candidates, source)
	}
}
