package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestThesisScore(t *testing.T) {
	convey.Convey("Given a thesis supported by multiple measurements", t, func() {
		measurements := []perspectives.Measurement{
			{Category: perspectives.CategoryAggressiveDrive, SNR: 2.0},
			{Category: perspectives.CategoryAggressiveDrive, SNR: 4.0},
		}

		convey.Convey("It should score root-mean-square signal energy", func() {
			score := thesisScore(measurements, []string{"drive"})

			convey.So(score, convey.ShouldBeGreaterThan, 3.0)
			convey.So(score, convey.ShouldBeLessThan, 3.2)
		})

		convey.Convey("It should rise when more relevant categories contribute SNR", func() {
			measurements = append(measurements, perspectives.Measurement{
				Category: perspectives.CategoryFrenzy,
				SNR:      5.0,
			})
			plain := thesisScore(measurements, []string{"drive"})
			confirmed := thesisScore(measurements, []string{"drive", "trend"})

			convey.So(confirmed, convey.ShouldBeGreaterThan, plain)
		})
	})
}

func TestRobustCenter(t *testing.T) {
	convey.Convey("Given a cross-section with one outlier", t, func() {
		median, spread := robustCenter([]float64{0.2, 0.2, 0.2, 3.0})

		convey.Convey("It should keep the center anchored to the current crowd", func() {
			convey.So(median, convey.ShouldEqual, 0.2)
			convey.So(spread, convey.ShouldEqual, 0)
		})
	})
}

func TestEntryDecisionContext(t *testing.T) {
	convey.Convey("Given maker entries are enabled", t, func() {
		originalUseMaker := config.System.UseMakerEntries
		originalMakerFee := config.System.MakerFeePct
		originalTakerFee := config.System.TakerFeePct
		originalEdge := config.System.EntryEdgeMultiple
		config.System.UseMakerEntries = true
		config.System.MakerFeePct = 0.25
		config.System.TakerFeePct = 0.40
		config.System.EntryEdgeMultiple = 3
		previousCatalog := market.Catalog()
		market.SetCatalog(nil)
		t.Cleanup(func() {
			config.System.UseMakerEntries = originalUseMaker
			config.System.MakerFeePct = originalMakerFee
			config.System.TakerFeePct = originalTakerFee
			config.System.EntryEdgeMultiple = originalEdge
			market.SetCatalog(previousCatalog)
			config.SyncRuntime()
		})
		config.SyncRuntime()

		crypto := newTestCrypto()
		crypto.runtime = config.Runtime
		context := crypto.entryDecisionContext("BTC/EUR", nil, "drive", 0)

		convey.Convey("It should score maker entry and taker exit friction", func() {
			takerTaker := perspectives.RequiredThesisScoreForFees(3, 0.40, 0.40, 0)

			convey.So(context.Metrics[perspectives.MetricFeePct], convey.ShouldEqual, 0.25)
			convey.So(context.Metrics[perspectives.MetricRequiredScore], convey.ShouldAlmostEqual, 1.53)
			convey.So(context.Metrics[perspectives.MetricRequiredScore], convey.ShouldBeLessThan, takerTaker)
		})
	})
}

func BenchmarkThesisScore(b *testing.B) {
	measurements := []perspectives.Measurement{
		{Category: perspectives.CategoryAggressiveDrive, SNR: 2.0},
		{Category: perspectives.CategoryAggressiveDrive, SNR: 4.0},
	}

	for b.Loop() {
		_ = thesisScore(measurements, []string{"drive", "trend"})
	}
}
