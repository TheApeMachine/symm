package correlation

import (
	"context"
	"iter"
	"math"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

var correlationCategories = []logic.CategoryType{
	logic.CategorySystemicHerd,
	logic.CategoryDecoupledAlpha,
	logic.CategoryStochasticNoise,
	logic.CategoryDivergentStress,
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given correlated cross-section returns", testingTB, func() {
		crossSection := testCrossSection(testingTB)
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
				datapoint := tickerFrame(at, symbol, last, changePct)
				datapoint.WithScope(symbol)

				packed := datapoint.Pack()
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "timestamp"), packed)
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "scope", "timestamp"), packed)

				observePeers(crossSection, datapoint)
				measured := firstMeasured(signal.Measure(datapoint, crossSection))

				if measured != nil {
					signal.tree = storeMeasurement(signal.tree, measured)
					result = measured
				}

				datapoint.Release()
			}
		}

		Convey("It should emit non-uniform cohort classification", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)
			So(datura.Peek[float64](result, "output", "peakScore"), ShouldNotBeNil)

			So(distributionSum(result, correlationCategories), ShouldAlmostEqual, 1, 0.0001)
			So(dominantCategoryIndex(result, correlationCategories), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a decoupled high-energy symbol", testingTB, func() {
		crossSection := testCrossSection(testingTB)
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
			"BTC/USD": {0.012, -0.010, 0.011, -0.009, 0.012, -0.010, 0.011, -0.009},
			"ETH/USD": {0.011, -0.009, 0.012, -0.010, 0.011, -0.009, 0.012, -0.010},
			"SOL/USD": {0.013, -0.011, 0.010, -0.008, 0.013, -0.011, 0.010, -0.008},
			"ADA/USD": {0.010, -0.008, 0.011, -0.009, 0.010, -0.008, 0.011, -0.009},
		}
		alphaReturns := []float64{0.180, 0.160, -0.140, -0.120, 0.170, 0.150, -0.130, -0.110}
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
			alphaCycle := tick % len(alphaReturns)

			for _, symbol := range peers {
				returnRate := peerReturns[symbol][cycle]
				peerLast[symbol] *= 1 + returnRate
				datapoint := tickerFrame(at, symbol, peerLast[symbol], returnRate*100)
				datapoint.WithScope(symbol)

				packed := datapoint.Pack()
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "timestamp"), packed)
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "scope", "timestamp"), packed)

				observePeers(crossSection, datapoint)
				_ = firstMeasured(signal.Measure(datapoint, crossSection))
				datapoint.Release()
			}

			alphaReturn := alphaReturns[alphaCycle]
			alphaLast *= 1 + alphaReturn
			datapoint := tickerFrame(at, "ALPHA/USD", alphaLast, alphaReturn*100)
			datapoint.WithScope("ALPHA/USD")

			packed := datapoint.Pack()
			signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "timestamp"), packed)
			signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "scope", "timestamp"), packed)

			observePeers(crossSection, datapoint)
			measured := firstMeasured(signal.Measure(datapoint, crossSection))

			if measured != nil {
				signal.tree = storeMeasurement(signal.tree, measured)

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
			So(dominantCategoryIndex(result, decoupledCategories),
				ShouldEqual, logic.CategoryIndex(logic.CategoryDecoupledAlpha))
			So(datura.Peek[float64](result, "output", "alphaScore"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "alphaScore"), ShouldBeGreaterThan,
				datura.Peek[float64](result, "output", "herdScore"))
			So(datura.Peek[float64](result, "output", "alphaScore"), ShouldBeGreaterThan,
				datura.Peek[float64](result, "output", "stressScore"))
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)

			result.Release()
		})
	})

	Convey("Given asynchronous returns where ticks do not align perfectly", testingTB, func() {
		crossSection := testCrossSection(testingTB)
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
			dpA := tickerFrame(atA.UnixNano(), "ASYNC_A/USD", valA, 5.0)
			dpA.WithScope("ASYNC_A/USD")
			packedA := dpA.Pack()
			signal.tree, _, _ = signal.tree.Insert(dpA.Prefix("role", "timestamp"), packedA)
			signal.tree, _, _ = signal.tree.Insert(dpA.Prefix("role", "scope", "timestamp"), packedA)
			observePeers(crossSection, dpA)
			_ = firstMeasured(signal.Measure(dpA, crossSection))
			dpA.Release()

			atB := base.Add(time.Duration(2*i) * time.Second)
			valB := 20.0 + float64(i)*1.0
			dpB := tickerFrame(atB.UnixNano(), "ASYNC_B/USD", valB, 5.0)
			dpB.WithScope("ASYNC_B/USD")
			packedB := dpB.Pack()
			signal.tree, _, _ = signal.tree.Insert(dpB.Prefix("role", "timestamp"), packedB)
			signal.tree, _, _ = signal.tree.Insert(dpB.Prefix("role", "scope", "timestamp"), packedB)
			observePeers(crossSection, dpB)
			measured := firstMeasured(signal.Measure(dpB, crossSection))
			if measured != nil {
				signal.tree = storeMeasurement(signal.tree, measured)
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
		crossSection := testCrossSection(testingTB)
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
			datapoint := tickerFrame(
				at,
				"BTC/USD", lasts["BTC/USD"], 1,
				"ETH/USD", lasts["ETH/USD"], 1,
				"SOL/USD", lasts["SOL/USD"], 1,
			)

			observePeers(crossSection, datapoint)

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
		crossSection := testCrossSection(b)
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		for tick := range 8 {
			at := base.Add(time.Duration(tick) * 10 * time.Second).UnixNano()

			for symbolIndex, symbol := range symbols {
				datapoint := tickerFrame(at, symbol, 100+float64(tick)+float64(symbolIndex), 0.5)
				datapoint.WithScope(symbol)

				packed := datapoint.Pack()
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "timestamp"), packed)
				signal.tree, _, _ = signal.tree.Insert(datapoint.Prefix("role", "scope", "timestamp"), packed)

				observePeers(crossSection, datapoint)
				_ = firstMeasured(signal.Measure(datapoint, crossSection))
				datapoint.Release()
			}
		}

		_ = signal.Close()
	}
}

func testCrossSection(testingTB testing.TB) *market.CrossSection {
	if testingTB != nil {
		testingTB.Helper()
	}

	crossSection, err := market.NewCrossSection(
		market.CrossSectionConfig{
			ReturnCap:  16,
			MinBars:    6,
			BreadthCap: 16,
		},
	)

	if err != nil && testingTB != nil {
		testingTB.Fatal(err)
	}

	return crossSection
}

func tickerFrame(timestamp int64, values ...any) *datura.Artifact {
	rows := make([]datura.Map[any], 0, len(values)/3)

	for index := 0; index < len(values); index += 3 {
		rows = append(rows, datura.Map[any]{
			"symbol":     values[index].(string),
			"last":       testNumber(values[index+1]),
			"volume":     1000.0,
			"change_pct": testNumber(values[index+2]),
		})
	}

	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload(datura.Map[any]{
		"channel": "ticker",
		"type":    "update",
		"data":    rows,
	}.Marshal())
	artifact.SetTimestamp(timestamp)

	return artifact
}

func testNumber(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		panic("ticker fixture value must be numeric")
	}
}

func observePeers(crossSection *market.CrossSection, datapoint *datura.Artifact) {
	var tickers kraken.TickerDataSlice

	for rowIndex := 0; ; rowIndex++ {
		symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")
		if symbol == "" {
			break
		}

		tickers = append(tickers, kraken.TickerData{
			Symbol:    symbol,
			Last:      datura.Peek[float64](datapoint, "data", rowIndex, "last"),
			Volume:    datura.Peek[float64](datapoint, "data", rowIndex, "volume"),
			ChangePct: datura.Peek[float64](datapoint, "data", rowIndex, "change_pct"),
			Timestamp: time.Unix(0, datapoint.Timestamp()).UTC(),
		})
	}

	_ = crossSection.Observe(tickers)
}

func firstMeasured(measurements iter.Seq[*datura.Artifact]) *datura.Artifact {
	for measurement := range measurements {
		return measurement
	}

	return nil
}

func storeMeasurement(tree *dmt.Tree, measurement *datura.Artifact) *dmt.Tree {
	if measurement == nil {
		return tree
	}

	updated, _, _ := tree.InsertArtifact(measurement.Prefix(), measurement)

	if updated == nil {
		return tree
	}

	return updated
}

func categoryMass(result *datura.Artifact, category logic.CategoryType) float64 {
	distribution := datura.Peek[map[string]any](result, "output", "distribution")
	mass, _ := distribution[strconv.Itoa(logic.CategoryIndex(category))].(float64)

	return mass
}

func dominantCategoryIndex(
	result *datura.Artifact,
	categories []logic.CategoryType,
) int {
	best := categories[0]
	bestMass := categoryMass(result, best)

	for _, category := range categories[1:] {
		mass := categoryMass(result, category)

		if mass > bestMass {
			best = category
			bestMass = mass
		}
	}

	return logic.CategoryIndex(best)
}

func distributionSum(result *datura.Artifact, categories []logic.CategoryType) float64 {
	total := 0.0

	for _, category := range categories {
		total += categoryMass(result, category)
	}

	return total
}
