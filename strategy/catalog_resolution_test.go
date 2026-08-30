package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
TestCatalogCoordinateUnitResolution proves the exact producer unit/timescale
identity for the coordinates whose unit was previously mislabeled as
"events per second". The producers emit data.UnitPerSecond ("per_second") /
data.TimescalePerSecond, which the ParseUnit map resolves to nmtypes.UnitPerSecond
— NOT UnitEventsPerSecond (that maps the distinct "events_per_second" spelling).
A catalog selector that names a rate coordinate under a different unit than the
producer would fail this test, so it fails when a coordinate is re-labeled
incorrectly.
*/
func TestCatalogCoordinateUnitResolution(t *testing.T) {
	Convey("Given the typed unit map", t, func() {
		Convey("the generic per-second rate parses to UnitPerSecond, not events", func() {
			So(nmtypes.ParseUnit("per_second"), ShouldEqual, nmtypes.UnitPerSecond)
			So(nmtypes.ParseUnit("events_per_second"), ShouldEqual, nmtypes.UnitEventsPerSecond)
			So(nmtypes.UnitPerSecond.String(), ShouldEqual, "per_second")
			So(nmtypes.UnitEventsPerSecond.String(), ShouldEqual, "events_per_second")
		})

		Convey("every rate coordinate in the catalog resolves to the producer's exact unit", func() {
			// The producers emit data.UnitPerSecond ("per_second") for these
			// rate metrics, so their catalog unit must be UnitPerSecond.
			rateCoords := []struct {
				source, metric, side string
				timescale            nmtypes.Timescale
			}{
				{"hawkes", "background_rate", "", nmtypes.TimescalePerSecond},
				{"hawkes", "conditional_intensity", "buy", nmtypes.TimescalePerSecond},
				{"hawkes", "conditional_intensity", "sell", nmtypes.TimescalePerSecond},
				{"derivatives", "liquidation_notional_rate", "", nmtypes.TimescalePerSecond},
				{"depthflow", "book_turnover_rate", "", nmtypes.TimescalePerSecond},
				{"toxicity", "retreat_rate", "ask", nmtypes.TimescaleInstantaneous},
			}

			for _, coordinate := range rateCoords {
				spec := catalogSpec(coordinate.source, coordinate.metric, coordinate.side)
				So(spec, ShouldNotEqual, MarketCoordinateSpec{})
				So(spec.Unit, ShouldEqual, nmtypes.UnitPerSecond)
				So(spec.Timescale, ShouldEqual, coordinate.timescale)
			}
		})

		Convey("no coordinate in the catalog remains labeled events-per-second", func() {
			for _, spec := range defaultMarketCatalog {
				So(spec.Unit, ShouldNotEqual, nmtypes.UnitEventsPerSecond)
			}
		})
	})
}

/*
TestCatalogExcludesPumpDumpSpread proves pumpdump/relative_spread was removed
from the curated causal schema: duplicate touch-spread context belongs to
Liquidity, and PumpDump's useful information is volume-clock activity (moved to
the Activity Perspective). No other PumpDump coordinate takes its place in MCTS.
*/
func TestCatalogExcludesPumpDumpSpread(t *testing.T) {
	Convey("Given the market catalog", t, func() {
		Convey("pumpdump/relative_spread is not a declared coordinate", func() {
			So(catalogSpec("pumpdump", "relative_spread", ""), ShouldEqual, MarketCoordinateSpec{})
		})

		Convey("no pumpdump coordinate remains in the catalog at all", func() {
			for _, spec := range defaultMarketCatalog {
				So(spec.Selector.Source, ShouldNotEqual, "pumpdump")
			}
		})
	})
}
