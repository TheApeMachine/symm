package exhaust

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/testutil"
)

var exhaustCategories = []logic.CategoryType{
	logic.CategoryMechanicalCollapse,
	logic.CategoryFragileExpansion,
	logic.CategoryThermalExhaustion,
	logic.CategoryActiveReversal,
}

func categoryResult(result *datura.Artifact) int {
	return testutil.DominantCategoryIndex(result, exhaustCategories)
}

var classifierInputs = []string{"mechanical", "fragile", "thermal", "reversal"}

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

func bookDatapoint(bidQty, askQty float64, timestamp int64) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":%g}],"asks":[{"price":101.0,"qty":%g}]}]}`,
		bidQty, askQty,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("book")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

func tradeDatapoint(side string, price, quantity float64, timestamp int64) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":%q,"price":%g,"qty":%g,"timestamp":"2026-05-30T12:00:00Z"}]}`,
		side, price, quantity,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("trade")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	Convey("Given crumbling bid-side depth", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		bidDepths := []float64{40, 32, 24, 16, 10, 6, 3, 1}
		var (
			result         *datura.Artifact
			bestMechanical float64
		)

		for index, bidQty := range bidDepths {
			datapoint := bookDatapoint(bidQty, 10, base.Add(time.Duration(index)*time.Second).UnixNano())
			measured := testutil.FirstMeasured(signal.Measure(datapoint, nil))

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)
				mechanicalScore := outputScore(measured, "mechanical")

				if mechanicalScore > bestMechanical {
					if winningClassifierInput(measured) != "mechanical" {
						measured.Release()

						datapoint.Release()

						continue
					}
					if result != nil {
						result.Release()
					}

					result = measured
					bestMechanical = mechanicalScore

					datapoint.Release()

					continue
				}

				measured.Release()
			}

			datapoint.Release()
		}

		Convey("It should classify mechanical collapse with mechanical winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryMechanicalCollapse))
			So(winningClassifierInput(result), ShouldEqual, "mechanical")
			So(outputScore(result, "mechanical"), ShouldBeGreaterThan, 0.25)

			result.Release()
		})
	})

	Convey("Given fading aggressive buy pressure", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		var result *datura.Artifact

		for index := range 12 {
			at := base.Add(time.Duration(index) * time.Second).UnixNano()
			bookFrame := bookDatapoint(10, 10, at)
			measured := testutil.FirstMeasured(signal.Measure(bookFrame, nil))

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)
			}

			bookFrame.Release()
		}

		tradeSizes := []float64{20, 18, 16, 14, 12, 10, 8, 6, 4, 2, 1, 0.5}
		fadeSizes := []float64{18, 14, 10, 6, 3}
		var bestThermal float64

		for index, quantity := range tradeSizes {
			at := base.Add(time.Duration(index+12) * time.Second).UnixNano()
			tradeFrame := tradeDatapoint("buy", 100, quantity, at)
			tradeMeasured := testutil.FirstMeasured(signal.Measure(tradeFrame, nil))

			if tradeMeasured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, tradeMeasured)
				tradeMeasured.Release()
			}

			tradeFrame.Release()
		}

		for index, quantity := range fadeSizes {
			at := base.Add(time.Duration(index+24) * time.Second).UnixNano()
			tradeFrame := tradeDatapoint("sell", 100, quantity, at)
			tradeMeasured := testutil.FirstMeasured(signal.Measure(tradeFrame, nil))

			if tradeMeasured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, tradeMeasured)

				if !testutil.HasConfidence(tradeMeasured) {
					tradeMeasured.Release()

					tradeFrame.Release()

					continue
				}

				thermalScore := outputScore(tradeMeasured, "thermal")

				if thermalScore > bestThermal {
					if result != nil {
						result.Release()
					}

					result = tradeMeasured
					bestThermal = thermalScore

					tradeFrame.Release()

					continue
				}

				tradeMeasured.Release()
			}

			tradeFrame.Release()
		}

		Convey("It should classify thermal exhaustion with thermal winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryThermalExhaustion))
			So(winningClassifierInput(result), ShouldEqual, "thermal")

			result.Release()
		})
	})

	Convey("Given stable book depth after warmup", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()
		var lastResult *datura.Artifact

		for index := range 12 {
			datapoint := bookDatapoint(10, 10, base+int64(index))
			lastResult = testutil.FirstMeasured(signal.Measure(datapoint, nil))
			signal.tree = testutil.StoreMeasurement(signal.tree, lastResult)
			datapoint.Release()
		}

		Convey("It should return a state seed without classifier output on zero decay urgency", func() {
			So(lastResult, ShouldNotBeNil)
			So(testutil.HasConfidence(lastResult), ShouldBeFalse)

			lastResult.Release()
		})
	})
}

func TestSignalColdStartRebuildsFromTree(testingTB *testing.T) {
	Convey("Given book and trade measurements written to a shared tree", testingTB, func() {
		tree := dmt.NewTree("")
		warm := NewSignal(context.Background(), tree)

		defer func() {
			_ = warm.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		bidDepths := []float64{40, 32, 24, 16, 10}

		for index, bidQty := range bidDepths {
			at := base.Add(time.Duration(index) * time.Second).UnixNano()
			frame := bookDatapoint(bidQty, 10, at)
			measured := testutil.FirstMeasured(warm.Measure(frame, nil))

			if measured != nil {
				tree = testutil.StoreMeasurement(warm.tree, measured)
				measured.Release()
			}

			frame.Release()
		}

		tradeSizes := []float64{20, 16, 12, 8, 4}

		for index, quantity := range tradeSizes {
			at := base.Add(time.Duration(index+5) * time.Second).UnixNano()
			frame := tradeDatapoint("buy", 100, quantity, at)
			measured := testutil.FirstMeasured(warm.Measure(frame, nil))

			if measured != nil {
				tree = testutil.StoreMeasurement(warm.tree, measured)
				measured.Release()
			}

			frame.Release()
		}

		Convey("A fresh Signal rebuilds book and trade streams separately from the tree", func() {
			cold := NewSignal(context.Background(), tree)

			defer func() {
				_ = cold.Close()
			}()

			depthDrops, _, _, prevDepth, _, _ := cold.bookHistory("BTC/USD")
			fadeHistory, _ := cold.tradeHistory("BTC/USD")

			So(prevDepth, ShouldBeGreaterThan, 0)
			So(len(depthDrops), ShouldBeGreaterThan, 0)
			So(len(fadeHistory), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		for index := range 8 {
			datapoint := bookDatapoint(20-float64(index)*2, 10, base+int64(index))
			measured := testutil.FirstMeasured(signal.Measure(datapoint, nil))
			signal.tree = testutil.StoreMeasurement(signal.tree, measured)
			datapoint.Release()
		}

		_ = signal.Close()
	}
}
