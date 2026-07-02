package correlation

import (
	"context"
	"fmt"
	"math"
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
				last := (100 + float64(symbolIndex)) * math.Pow(1.2, float64(tick))
				datapoint := testutil.TickerDatapoint(symbol, last, changePct, at)
				datapoint.WithScope(symbol)

				packed := datapoint.Pack()
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "timestamp"), packed)
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "scope", "timestamp"), packed)

				testutil.ObservePeers(crossSection, datapoint)
				measured := testutil.FirstMeasured(signal.Measure(datapoint, crossSection))

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
			So(datura.Peek[float64](result, "output", "peakScore"), ShouldNotBeNil)

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
			"BTC/USD": {0.050, -0.040, 0.050, -0.040, 0.050, -0.040, 0.050, -0.040},
			"ETH/USD": {-0.040, 0.050, -0.040, 0.050, -0.040, 0.050, -0.040, 0.050},
			"SOL/USD": {0.040, 0.040, -0.030, -0.030, 0.040, 0.040, -0.030, -0.030},
			"ADA/USD": {-0.030, -0.030, 0.040, 0.040, -0.030, -0.030, 0.040, 0.040},
		}
		alphaReturns := []float64{0.180, -0.020, -0.150, 0.030, 0.160, -0.040, -0.120, 0.020, 0.140, -0.030}
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
				datapoint.WithScope(symbol)

				packed := datapoint.Pack()
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "timestamp"), packed)
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "scope", "timestamp"), packed)

				testutil.ObservePeers(crossSection, datapoint)
				_ = testutil.FirstMeasured(signal.Measure(datapoint, crossSection))
				datapoint.Release()
			}

			alphaReturn := alphaReturns[alphaCycle]
			alphaLast *= 1 + alphaReturn
			datapoint := testutil.TickerDatapoint("ALPHA/USD", alphaLast, alphaReturn*100, at)
			datapoint.WithScope("ALPHA/USD")

			packed := datapoint.Pack()
			signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "timestamp"), packed)
			signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "scope", "timestamp"), packed)

			testutil.ObservePeers(crossSection, datapoint)
			measured := testutil.FirstMeasured(signal.Measure(datapoint, crossSection))

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

	Convey("Given asynchronous returns where ticks do not align perfectly", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		var result *datura.Artifact

		for i := 1; i <= 10; i++ {
			atA := base.Add(time.Duration(2*i-1) * time.Second)
			valA := 10.0 + float64(i)*0.5
			dpA := testutil.TickerDatapoint("ASYNC_A/USD", valA, 5.0, atA.UnixNano())
			dpA.WithScope("ASYNC_A/USD")
			packedA := dpA.Pack()
			signal.tree, _, _ = signal.tree.Insert(dpA.Prefix("role", "timestamp"), packedA)
			signal.tree, _, _ = signal.tree.Insert(dpA.Prefix("role", "scope", "timestamp"), packedA)
			testutil.ObservePeers(crossSection, dpA)
			_ = testutil.FirstMeasured(signal.Measure(dpA, crossSection))
			dpA.Release()

			atB := base.Add(time.Duration(2*i) * time.Second)
			valB := 20.0 + float64(i)*1.0
			dpB := testutil.TickerDatapoint("ASYNC_B/USD", valB, 5.0, atB.UnixNano())
			dpB.WithScope("ASYNC_B/USD")
			packedB := dpB.Pack()
			signal.tree, _, _ = signal.tree.Insert(dpB.Prefix("role", "timestamp"), packedB)
			signal.tree, _, _ = signal.tree.Insert(dpB.Prefix("role", "scope", "timestamp"), packedB)
			testutil.ObservePeers(crossSection, dpB)
			measured := testutil.FirstMeasured(signal.Measure(dpB, crossSection))
			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)
				if result != nil {
					result.Release()
				}
				result = measured
			}
			dpB.Release()
		}

		Convey("It should use cross-section peer returns and report high correlation", func() {
			So(result, ShouldNotBeNil)
			corr := datura.Peek[float64](result, "output", "correlation")
			So(corr, ShouldBeGreaterThan, 0.8)
			So(datura.Peek[float64](result, "output", "peakScore"), ShouldNotBeNil)
			result.Release()
		})
	})
}

func TestMeasureUsesEveryTickerRow(testingTB *testing.T) {
	Convey("Given multi-row ticker artifacts", testingTB, func() {
		crossSection := testutil.NewTestCrossSection(testingTB)
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		lasts := map[string]float64{
			"BTC/USD": 100,
			"ETH/USD": 80,
			"SOL/USD": 40,
		}
		returns := map[string][]float64{
			"BTC/USD": {0.010, -0.004, 0.013, 0.002, 0.009, -0.003, 0.011, 0.004},
			"ETH/USD": {0.008, -0.003, 0.011, 0.003, 0.007, -0.002, 0.010, 0.003},
			"SOL/USD": {0.020, 0.006, -0.012, 0.018, -0.007, 0.016, -0.010, 0.014},
		}
		var thirdRowMeasured *datura.Artifact

		for tick := range 8 {
			for symbol, series := range returns {
				lasts[symbol] *= 1 + series[tick]
			}

			at := base.Add(time.Duration(tick) * time.Second).UnixNano()
			datapoint := datura.Acquire("kraken:public", datura.APPJSON)
			datapoint.WithRole("ticker")
			datapoint.WithScope("update")
			datapoint.WithPayload([]byte(fmt.Sprintf(
				`{"channel":"ticker","type":"update","data":[`+
					`{"symbol":"BTC/USD","last":%f,"volume":1000,"change_pct":1},`+
					`{"symbol":"ETH/USD","last":%f,"volume":900,"change_pct":1},`+
					`{"symbol":"SOL/USD","last":%f,"volume":800,"change_pct":1}`+
					`]}`,
				lasts["BTC/USD"],
				lasts["ETH/USD"],
				lasts["SOL/USD"],
			)))
			datapoint.SetTimestamp(at)

			testutil.ObservePeers(crossSection, datapoint)

			for measurement := range signal.Measure(datapoint, crossSection) {
				scope, _ := measurement.Scope()

				if scope != "SOL/USD" {
					measurement.Release()
					continue
				}

				if thirdRowMeasured != nil {
					thirdRowMeasured.Release()
				}

				thirdRowMeasured = measurement
			}

			datapoint.Release()
		}

		Convey("It should measure rows beyond data[0]", func() {
			So(thirdRowMeasured, ShouldNotBeNil)
			So(datura.Peek[float64](thirdRowMeasured, "output", "confidence"), ShouldBeGreaterThan, 0)
			thirdRowMeasured.Release()
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
				datapoint.WithScope(symbol)

				packed := datapoint.Pack()
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "timestamp"), packed)
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "scope", "timestamp"), packed)

				testutil.ObservePeers(crossSection, datapoint)
				_ = testutil.FirstMeasured(signal.Measure(datapoint, crossSection))
				datapoint.Release()
			}
		}

		_ = signal.Close()
	}
}
