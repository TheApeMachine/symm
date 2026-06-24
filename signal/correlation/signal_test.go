package correlation

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	marketsection "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/testutil"
)

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	if testingTB != nil {
		testingTB.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && testingTB != nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func newTestCrossSection(testingTB testing.TB) *marketsection.CrossSection {
	if testingTB != nil {
		testingTB.Helper()
	}

	section, err := marketsection.NewCrossSection(&marketsection.CrossSectionConfig{
		ReturnCap:   16,
		MinBars:     6,
		BreadthHist: 16,
	})

	if err != nil && testingTB != nil {
		testingTB.Fatal(err)
	}

	return section
}

func tickerDatapoint(symbol string, last, changePct float64, timestamp int64) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":%g,"volume":1000,"change_pct":%g}]}`,
		symbol, last, changePct,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

var correlationCategories = []logic.CategoryType{
	logic.CategorySystemicHerd,
	logic.CategoryDecoupledAlpha,
	logic.CategoryStochasticNoise,
	logic.CategoryDivergentStress,
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given correlated cross-section returns", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
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
				datapoint := tickerDatapoint(symbol, last, changePct, at)
				measured := signal.Measure(datapoint)

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
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""), newTestCrossSection(testingTB))
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
			"BTC/USD": {0.010, 0.012, 0.011, 0.013, 0.010, 0.012, 0.011, 0.013},
			"ETH/USD": {0.012, 0.010, 0.013, 0.011, 0.012, 0.010, 0.013, 0.011},
			"SOL/USD": {0.011, 0.013, 0.010, 0.012, 0.011, 0.013, 0.010, 0.012},
			"ADA/USD": {0.013, 0.011, 0.012, 0.010, 0.013, 0.011, 0.012, 0.010},
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
				datapoint := tickerDatapoint(symbol, peerLast[symbol], returnRate*100, at)
				_ = signal.Measure(datapoint)
				datapoint.Release()
			}

			alphaReturn := alphaReturns[alphaCycle]
			alphaLast *= 1 + alphaReturn
			datapoint := tickerDatapoint("ALPHA/USD", alphaLast, alphaReturn*100, at)
			measured := signal.Measure(datapoint)

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
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		for tick := range 8 {
			at := base.Add(time.Duration(tick) * 10 * time.Second).UnixNano()

			for symbolIndex, symbol := range symbols {
				datapoint := tickerDatapoint(symbol, 100+float64(tick)+float64(symbolIndex), 0.5, at)
				_ = signal.Measure(datapoint)
				datapoint.Release()
			}
		}

		_ = signal.Close()
	}
}
