package fluid

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

func measureTickerFrame(signal *Signal, symbol string, volume, last, bid, ask float64) *datura.Artifact {
	replaySequence := time.Now().UnixNano()

	stored := datura.Acquire("kraken:public", datura.APPJSON)
	stored.WithRole("ticker")
	stored.WithScope("update")
	stored.WithPayload(tickerFrame(symbol, volume, last, bid, ask))
	stored.SetTimestamp(replaySequence)

	result := signal.Measure(stored, nil)
	signal.tree = testutil.StoreMeasurement(signal.tree, result)

	return result
}

func warmupStableTicker(signal *Signal, symbol string, tickCount int) {
	for tick := range tickCount {
		replaySequence := time.Now().UnixNano() + int64(tick)*int64(time.Millisecond)

		volume := 1000.0 + float64(tick)
		stored := datura.Acquire("kraken:public", datura.APPJSON)
		stored.WithRole("ticker")
		stored.WithScope("update")
		stored.WithPayload(tickerFrame(symbol, volume, 100, 99.99, 100.01))
		stored.SetTimestamp(replaySequence)

		result := signal.Measure(stored, nil)
		signal.tree = testutil.StoreMeasurement(signal.tree, result)

		if result != nil {
			result.Release()
		}
	}
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given a warmed tight-spread stable ticker", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		symbol := "ETH/EUR"
		warmupStableTicker(signal, symbol, 60)
		result := measureTickerFrame(signal, symbol, 1060, 100, 99.99, 100.01)

		Convey("It should classify laminar stability with laminar winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "laminar"), ShouldBeGreaterThan, 0)
			So(outputScore(result, "laminar"), ShouldBeGreaterThan, outputScore(result, "turbulent"))
			So(winningClassifierInput(result), ShouldEqual, "laminar")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryLaminar))
			So(testutil.DistributionSum(result, fluidCategories), ShouldAlmostEqual, 1, 0.0001)

			result.Release()
		})
	})
}
