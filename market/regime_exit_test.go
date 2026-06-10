package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestSymbolExitMoves(t *testing.T) {
	Convey("Given enough return history", t, func() {
		configureRegimeViper()
		viper.Set("signals.causal.contagion_window_slow_max", 128)
		viper.Set("signals.causal.contagion_window_slow_min", 16)

		crossSection, err := NewCrossSection(&CrossSectionConfig{
			MatchWindow: time.Minute,
			ReturnCap:   64,
			MinBars:     8,
			BreadthHist: 64,
		})
		So(err, ShouldBeNil)

		classifier, err := NewRegimeClassifier(crossSection)
		So(err, ShouldBeNil)

		eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		viper.Set("regime.window", 8)

		for index, multiplier := range []float64{1.0, 1.01, 1.02, 1.03, 1.04, 1.05, 1.06, 1.07, 1.08, 1.09, 1.10, 1.11, 1.12, 1.13, 1.14, 1.15, 1.16, 1.17} {
			crossSection.Observe(&krakenmarket.Symbol{
				Name:    "ETH/EUR",
				Price:   100 * multiplier,
				Updated: eventAt.Add(time.Duration(index) * time.Minute),
			})
		}

		stopPct, profitPct, ok := classifier.SymbolExitMoves("ETH/EUR")

		Convey("It should derive stop and profit moves from returns", func() {
			So(ok, ShouldBeTrue)
			So(stopPct, ShouldBeGreaterThan, 0)
			So(profitPct, ShouldBeGreaterThan, 0)
		})
	})
}
