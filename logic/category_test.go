package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestCategoryComposerPumpLifecycle(testingTB *testing.T) {
	Convey("Given pumpdump ignition dominates complementary measurements", testingTB, func() {
		at := time.Unix(10, 0)
		ignition := types.ObservationMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricIgnition,
			types.SubjectPumpIgnition, "BTC/USD", at,
			types.UnitDimensionless, 0.9, 0.8,
		)
		exhaustion := types.ObservationMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricExhaustion,
			types.SubjectPumpExhaustion, "BTC/USD", at,
			types.UnitDimensionless, 0.1, 0.8,
		)
		hawkes := types.ObservationMeasurement(
			types.SourceHawkes, types.Hawkes, types.MetricArrivalRate,
			types.SubjectTradeArrivals, "BTC/USD", at,
			types.UnitDimensionless, 2, 0.7,
		)
		thesis := types.NewThesis(nil)
		thesis.Measurements = append(thesis.Measurements, ignition, exhaustion, hawkes)
		analyzer := &Analyzer{}

		analyzer.Update(thesis)

		Convey("Then it should assign a traceable ignition category", func() {
			So(thesis.Categories, ShouldNotBeEmpty)

			var ignitionCategory types.Category

			for _, category := range thesis.Categories {
				if category.Type == types.CategoryVerticalIgnition {
					ignitionCategory = category
				}
			}

			So(ignitionCategory.Symbol, ShouldEqual, "BTC/USD")
			So(ignitionCategory.Strength, ShouldBeGreaterThan, 0.5)
			So(ignitionCategory.Maturity, ShouldEqual, 0.8)
			So(ignitionCategory.Missing, ShouldContain, string(types.SubjectPeerLiquidity))
		})
	})
}

func TestCategoryComposerToxicBluff(testingTB *testing.T) {
	Convey("Given asymmetric touch cancellation pressure", testingTB, func() {
		at := time.Unix(30, 0)
		thesis := types.NewThesis(nil)
		thesis.Measurements = append(thesis.Measurements,
			types.ObservationSideNormalizedMeasurement(
				types.SourceToxicity, types.Toxicity, types.MetricCancelledQuantity,
				types.SubjectLevel3Touch, "SOL/USD", types.SideBuy, at,
				types.UnitBaseCurrency, 4, 0.8, types.NormalizeFinite(0.8),
			),
			types.ObservationSideNormalizedMeasurement(
				types.SourceToxicity, types.Toxicity, types.MetricCancelledQuantity,
				types.SubjectLevel3Touch, "SOL/USD", types.SideSell, at,
				types.UnitBaseCurrency, 1, 0.8, types.NormalizeFinite(0.2),
			),
		)
		analyzer := &Analyzer{}

		analyzer.Update(thesis)

		Convey("Then it should classify toxic bluff from dominant retreat", func() {
			var bluff types.Category

			for _, category := range thesis.Categories {
				if category.Type == types.CategoryToxicBluff {
					bluff = category
				}
			}

			So(bluff.Symbol, ShouldEqual, "SOL/USD")
			So(bluff.Strength, ShouldBeGreaterThan, 0.5)
		})
	})
}

func TestCategoryComposerLiquidityVacuum(testingTB *testing.T) {
	Convey("Given peer scarcity dominates executable depth", testingTB, func() {
		at := time.Unix(20, 0)
		thesis := types.NewThesis(nil)
		thesis.Measurements = append(thesis.Measurements,
			types.ObservationMeasurement(
				types.SourceLiquidity, types.Liquidity, types.MetricScarcityScore,
				types.SubjectPeerLiquidity, "ETH/USD", at,
				types.UnitDimensionless, 0.9, 0.75,
			),
			types.ObservationMeasurement(
				types.SourceLiquidity, types.Liquidity, types.MetricDepthScore,
				types.SubjectPeerLiquidity, "ETH/USD", at,
				types.UnitDimensionless, 0.1, 0.75,
			),
		)
		analyzer := &Analyzer{}

		analyzer.Update(thesis)

		Convey("Then it should classify the liquidity vacuum with evidence gaps", func() {
			var vacuum types.Category

			for _, category := range thesis.Categories {
				if category.Type == types.CategoryLiquidityVacuum {
					vacuum = category
				}
			}

			So(vacuum.Symbol, ShouldEqual, "ETH/USD")
			So(vacuum.Strength, ShouldBeGreaterThan, 0.5)
			So(vacuum.Missing, ShouldContain, string(types.SubjectPumpVolumeLift))
		})
	})
}

func BenchmarkCategoryComposerCompose(benchmark *testing.B) {
	at := time.Unix(1, 0)
	measurements := []*types.Measurement{
		types.ObservationMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricIgnition,
			types.SubjectPumpIgnition, "BTC/USD", at,
			types.UnitDimensionless, 0.8, 0.9,
		),
		types.ObservationMeasurement(
			types.SourceLiquidity, types.Liquidity, types.MetricScarcityScore,
			types.SubjectPeerLiquidity, "BTC/USD", at,
			types.UnitDimensionless, 0.7, 0.6,
		),
		types.ObservationMeasurement(
			types.SourceLiquidity, types.Liquidity, types.MetricDepthScore,
			types.SubjectPeerLiquidity, "BTC/USD", at,
			types.UnitDimensionless, 0.2, 0.6,
		),
	}
	composer := CategoryComposer{}

	for benchmark.Loop() {
		thesis := types.NewThesis(nil)
		thesis.Measurements = measurements
		thesis.Graphs = map[string]*types.Graph{
			"BTC/USD": {},
		}
		composer.Compose(thesis, "BTC/USD")
	}
}
