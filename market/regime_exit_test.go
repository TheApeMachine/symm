package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal/testconfig"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestSymbolExitMoves(t *testing.T) {
	Convey("Given enough return history", t, func() {
		testconfig.SeedCompactRegime()
		regime, err := config.DerivedRegimeSpec()
		So(err, ShouldBeNil)
		crossSpec := config.DerivedCrossSectionSpec(regime)

		crossSection, err := NewCrossSection(&CrossSectionConfig{
			MatchWindow: crossSpec.MatchWindow,
			ReturnCap:   regime.Window,
			MinBars:     crossSpec.MinBars,
			BreadthHist: crossSpec.BreadthHist,
		})
		So(err, ShouldBeNil)

		classifier, err := NewRegimeClassifier(crossSection)
		So(err, ShouldBeNil)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		observationCount := regime.Window + regime.MinSamples

		for index := range observationCount {
			multiplier := 1.0 + float64(index)*0.01
			row, rowErr := krakenmarket.NewSymbolRow(
				"ETH/EUR",
				100*multiplier,
				0.01,
				10000,
				1,
				eventAt.Add(time.Duration(index)*time.Minute),
			)
			So(rowErr, ShouldBeNil)
			So(crossSection.Observe(row), ShouldBeNil)
		}

		stopPct, profitPct, ok := classifier.SymbolExitMoves("ETH/EUR")

		Convey("It should derive stop and profit moves from returns", func() {
			So(ok, ShouldBeTrue)
			So(stopPct, ShouldBeGreaterThan, 0)
			So(profitPct, ShouldBeGreaterThan, 0)
		})
	})
}
