package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
)

func TestSymbolsGroupBySymbol(t *testing.T) {
	Convey("Given aggregate and per-symbol layers on a tick", t, func() {
		val := data.NewValue("workspace", "tick", "aggregate")
		val.Append([]float64{0.005}, "liquidity", "aggregate", "market_friction")
		val.Append([]float64{0.8}, "correlation", "aggregate", "structural_alignment")
		val.Append([]float64{1.1}, "hawkes", "STALE", "USD", "supercritical_cascade")
		val.Append([]float64{1.2}, "hawkes", "LIVE", "USD", "supercritical_cascade")

		symbols := NewSymbols(val)
		bookStale := func(symbol string) bool { return symbol == "STALE/USD" }

		bySymbol := symbols.GroupBySymbol(bookStale)

		Convey("Stale pairs skip book-dependent aggregate fan-out", func() {
			So(bySymbol["STALE/USD"]["liquidity"], ShouldBeNil)
			So(bySymbol["STALE/USD"]["correlation"], ShouldNotBeNil)
			So(bySymbol["STALE/USD"]["hawkes"], ShouldNotBeNil)
		})

		Convey("Live pairs receive book-dependent aggregate fan-out", func() {
			So(bySymbol["LIVE/USD"]["liquidity"], ShouldNotBeNil)
		})
	})
}
