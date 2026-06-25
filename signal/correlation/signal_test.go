package correlation

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/testutil"
)

var correlationCategories = []logic.CategoryType{
	logic.CategorySystemicHerd,
	logic.CategoryDecoupledAlpha,
	logic.CategoryStochasticNoise,
	logic.CategoryDivergentStress,
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given correlated cross-section returns", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
		var result *datura.Artifact

		for tick := range 8 {
			at := base.Add(time.Duration(tick) * 10 * time.Second).UnixNano()
			changePct := 0.5 + float64(tick)*0.1

			for symbolIndex, symbol := range symbols {
				last := 100 + float64(tick) + float64(symbolIndex)*0.01
				datapoint := testutil.TickerDatapoint(symbol, last, changePct, at)
				measured := signal.Measure(datapoint, crossSection)

				if measured != nil {
					signal.tree = testutil.StoreMeasurement(signal.tree, measured)
					result = measured
				}

				datapoint.Release()
			}
		}

		Convey("It should emit non-uniform cohort classification", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)

			So(testutil.DistributionSum(result, correlationCategories), ShouldAlmostEqual, 1, 0.0001)
			So(testutil.DominantCategoryIndex(result, correlationCategories), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a decoupled high-energy symbol", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		peers := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}
		decoupledCategories := []logic.CategoryType{
			logic.CategorySystemicHerd,
			logic.CategoryDecoupledAlpha,
			logic.CategoryStochasticNoise,
			logic.CategoryDivergentStress,
		}
		peerReturns := map[string][]float64{
			"BTC/USD": {0.010, 0.015, 0.008, 0.020, 0.012, 0.009, 0.018, 0.011},
			"ETH/USD": {0.020, 0.005, 0.025, 0.007, 0.022, 0.004, 0.019, 0.006},
			"SOL/USD": {0.005, 0.025, 0.003, 0.028, 0.006, 0.024, 0.004, 0.027},
			"ADA/USD": {0.015, 0.002, 0.017, 0.001, 0.016, 0.003, 0.014, 0.002},
		}
		alphaReturns := []float64{0.200, 0.050, -0.080, 0.180, -0.020, 0.150, 0.100, -0.060, 0.120, 0.030}
		peerLast := map[string]float64{
			"BTC/USD": 100,
			"ETH/USD": 100,
			"SOL/USD": 100,
			"ADA/USD": 100,
		}
		alphaLast := 50.0
		var result *datura.Artifact

		for tick := range 24 {
			at := base.Add(time.Duration(tick) * time.Minute).UnixNano()
			cycle := tick % len(peerReturns["BTC/USD"])
			alphaCycle := (tick*3 + 1) % len(alphaReturns)

			for _, symbol := range peers {
				returnRate := peerReturns[symbol][cycle]
				peerLast[symbol] *= 1 + returnRate
				datapoint := testutil.TickerDatapoint(symbol, peerLast[symbol], returnRate*100, at)
				_ = signal.Measure(datapoint, crossSection)
				datapoint.Release()
			}

			alphaReturn := alphaReturns[alphaCycle]
			alphaLast *= 1 + alphaReturn
			datapoint := testutil.TickerDatapoint("ALPHA/USD", alphaLast, alphaReturn*100, at)
			measured := signal.Measure(datapoint, crossSection)

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)

				if result != nil {
					result.Release()
				}

				result = measured

				datapoint.Release()

				continue
			}

			datapoint.Release()
		}

		Convey("It should classify decoupled alpha with category 2", func() {
			So(result, ShouldNotBeNil)
			So(testutil.DominantCategoryIndex(result, decoupledCategories),
				ShouldEqual, logic.CategoryIndex(logic.CategoryDecoupledAlpha))
			So(datura.Peek[float64](result, "output", "alphaScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "alphaScore"), ShouldBeGreaterThan,
				datura.Peek[float64](result, "output", "herdScore"))
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)

			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}

	b.ReportAllocs()

	for b.Loop() {
		crossSection := testutil.NewTestCrossSection(b)
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		for tick := range 8 {
			at := base.Add(time.Duration(tick) * 10 * time.Second).UnixNano()

			for symbolIndex, symbol := range symbols {
				datapoint := testutil.TickerDatapoint(symbol, 100+float64(tick)+float64(symbolIndex), 0.5, at)
				_ = signal.Measure(datapoint, crossSection)
				datapoint.Release()
			}
		}

		_ = signal.Close()
	}
}
