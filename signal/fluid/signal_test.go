package fluid

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/testutil"
)

var fluidCategories = []logic.CategoryType{
	logic.CategoryLaminar,
	logic.CategoryTurbulent,
	logic.CategoryInertial,
	logic.CategoryViscous,
}

var classifierInputs = []string{"laminar", "turbulent", "inertial", "viscous"}

func categoryResult(result *datura.Artifact) int {
	return testutil.DominantCategoryIndex(result, fluidCategories)
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
	return fmt.Appendf(nil,
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"bid":%g,"bid_qty":5,"ask":%g,"ask_qty":5,"last":%g,"volume":%g}]}`,
		symbol, bid, ask, last, volume,
	)
}

func bookFrame(symbol, frameType string, bidQty, askQty float64) []byte {
	return fmt.Appendf(nil,
		`{"channel":"book","type":%q,"data":[{"symbol":%q,"bids":[{"price":99.99,"qty":%g},{"price":99.98,"qty":%g}],"asks":[{"price":100.01,"qty":%g},{"price":100.02,"qty":%g}]}]}`,
		frameType, symbol, bidQty, bidQty, askQty, askQty,
	)
}

func tradeFrame(symbol string, price, qty float64, side string) []byte {
	return fmt.Appendf(nil,
		`{"channel":"trade","type":"update","data":[{"symbol":%q,"side":%q,"price":%g,"qty":%g}]}`,
		symbol, side, price, qty,
	)
}

func measureFrame(signal *Signal, role string, payload []byte, at time.Time) *datura.Artifact {
	stored := datura.Acquire("kraken:public", datura.APPJSON)
	stored.WithRole(role)
	stored.WithScope("update")
	stored.WithPayload(payload)
	stored.SetTimestamp(at.UnixNano())

	result := testutil.FirstMeasured(signal.Measure(stored, nil))
	signal.tree = testutil.StoreMeasurement(signal.tree, result)

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
			So(testutil.DistributionSum(result, fluidCategories), ShouldAlmostEqual, 1, 0.0001)
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
