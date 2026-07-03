package fluid

import (
	"context"
	"encoding/json"
	"iter"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

var fluidCategories = []logic.CategoryType{
	logic.CategoryLaminar,
	logic.CategoryTurbulent,
	logic.CategoryInertial,
	logic.CategoryViscous,
}

var classifierInputs = []string{"laminar", "turbulent", "inertial", "viscous"}

func categoryResult(result *datura.Artifact) int {
	return dominantCategoryIndex(result, fluidCategories)
}

func outputScore(result *datura.Artifact, key string) float64 {
	return datura.Peek[float64](result, "output", key)
}

func winningClassifierInput(result *datura.Artifact) string {
	bestKey := classifierInputs[0]
	bestScore := outputScore(result, bestKey)

	for _, key := range classifierInputs[1:] {
		score := outputScore(result, key)

		if score > bestScore {
			bestScore = score
			bestKey = key
		}
	}

	return bestKey
}

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

func tickerFrame(symbol string, volume, last, bid, ask float64) []byte {
	return datura.Map[any]{
		"channel": "ticker",
		"type":    "update",
		"data": []datura.Map[any]{
			{
				"symbol":  symbol,
				"bid":     bid,
				"bid_qty": 5.0,
				"ask":     ask,
				"ask_qty": 5.0,
				"last":    last,
				"volume":  volume,
			},
		},
	}.Marshal()
}

func bookFrame(symbol, frameType string, bidQty, askQty float64) []byte {
	return datura.Map[any]{
		"channel": "book",
		"type":    frameType,
		"data": []datura.Map[any]{
			{
				"symbol": symbol,
				"bids": []datura.Map[any]{
					{"price": 99.99, "qty": bidQty},
					{"price": 99.98, "qty": bidQty},
				},
				"asks": []datura.Map[any]{
					{"price": 100.01, "qty": askQty},
					{"price": 100.02, "qty": askQty},
				},
			},
		},
	}.Marshal()
}

func tradeFrame(symbol string, price, qty float64, side string) []byte {
	return datura.Map[any]{
		"channel": "trade",
		"type":    "update",
		"data": []datura.Map[any]{
			{
				"symbol": symbol,
				"side":   side,
				"price":  price,
				"qty":    qty,
			},
		},
	}.Marshal()
}

func measureFrame(signal *Signal, role string, payload []byte, at time.Time) *datura.Artifact {
	stored := datura.Acquire("kraken:public", datura.APPJSON)
	stored.WithRole(role)
	stored.WithScope("update")
	stored.WithPayload(payload)
	stored.SetTimestamp(at.UnixNano())

	result := firstMeasured(signal.Measure(stored, nil))
	signal.tree = storeMeasurement(signal.tree, result)

	return result
}

func measureTickerFrame(signal *Signal, symbol string, volume, last, bid, ask float64, at time.Time) *datura.Artifact {
	return measureFrame(signal, "ticker", tickerFrame(symbol, volume, last, bid, ask), at)
}

func measureBookFrame(signal *Signal, symbol, frameType string, bidQty, askQty float64, at time.Time) *datura.Artifact {
	return measureFrame(signal, "book", bookFrame(symbol, frameType, bidQty, askQty), at)
}

func measureTradeFrame(signal *Signal, symbol string, price, qty float64, side string, at time.Time) *datura.Artifact {
	return measureFrame(signal, "trade", tradeFrame(symbol, price, qty, side), at)
}

func hasOutputKey(result *datura.Artifact, key string) bool {
	body := map[string]any{}

	if json.Unmarshal(result.DecryptPayload(), &body) != nil {
		return false
	}

	output, ok := body["output"].(map[string]any)

	if !ok {
		return false
	}

	_, ok = output[key]

	return ok
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given a tight-spread stable book and ticker", testingTB, func() {
		setFluidGridConfig()

		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		symbol := "ETH/EUR"
		start := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)

		So(measureTickerFrame(signal, symbol, 1000, 100, 99.99, 100.01, start), ShouldBeNil)
		So(measureBookFrame(signal, symbol, "snapshot", 5, 5, start.Add(10*time.Millisecond)), ShouldBeNil)

		result := measureBookFrame(signal, symbol, "update", 5, 5, start.Add(110*time.Millisecond))

		Convey("It should classify laminar stability with laminar winning", func() {
			So(result, ShouldNotBeNil)
			scope, err := result.Scope()
			So(err, ShouldBeNil)
			So(scope, ShouldEqual, symbol)
			So(outputScore(result, "laminar"), ShouldBeGreaterThan, 0)
			So(outputScore(result, "laminar"), ShouldBeGreaterThan, outputScore(result, "turbulent"))
			So(winningClassifierInput(result), ShouldEqual, "laminar")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryLaminar))
			So(distributionSum(result, fluidCategories), ShouldAlmostEqual, 1, 0.0001)
			So(hasOutputKey(result, "vorticity"), ShouldBeTrue)

			result.Release()
		})
	})
}

func TestSignalMeasureCarriesTradeIntoNextBookReading(testingTB *testing.T) {
	Convey("Given ticker, book, and trade flow for one symbol", testingTB, func() {
		setFluidGridConfig()

		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		symbol := "ETH/EUR"
		start := time.Date(2026, 6, 25, 12, 1, 0, 0, time.UTC)

		So(measureTickerFrame(signal, symbol, 1000, 100, 99.99, 100.01, start), ShouldBeNil)
		So(measureBookFrame(signal, symbol, "snapshot", 5, 5, start.Add(10*time.Millisecond)), ShouldBeNil)
		So(measureTradeFrame(signal, symbol, 100.01, 1, "buy", start.Add(20*time.Millisecond)), ShouldBeNil)

		result := measureBookFrame(signal, symbol, "update", 5, 4, start.Add(110*time.Millisecond))

		Convey("It should emit scoped mechanical metrics from the solver", func() {
			So(result, ShouldNotBeNil)
			So(hasOutputKey(result, "vorticity"), ShouldBeTrue)
			So(hasOutputKey(result, "viscosity"), ShouldBeTrue)
			So(hasOutputKey(result, "reynolds"), ShouldBeTrue)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)

			result.Release()
		})
	})
}

func TestSignalMeasureRequiresEventTimestamp(testingTB *testing.T) {
	Convey("Given a ticker frame without row or artifact timestamp", testingTB, func() {
		setFluidGridConfig()

		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		stored := datura.Acquire("kraken:public", datura.APPJSON)
		stored.WithRole("ticker")
		stored.WithScope("update")
		stored.WithPayload(tickerFrame("ETH/EUR", 1000, 100, 99.99, 100.01))
		stored.SetTimestamp(0)

		result := firstMeasured(signal.Measure(stored, nil))

		Convey("It should return an error artifact instead of inventing time", func() {
			So(result, ShouldNotBeNil)
			So(string(result.DecryptPayload()), ShouldContainSubstring, "fluid: event timestamp required")
		})
	})
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

func dominantCategoryIndex(result *datura.Artifact, categories []logic.CategoryType) int {
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
