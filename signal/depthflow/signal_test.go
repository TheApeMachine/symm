package depthflow

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

func marketDatapoint(channel, payload string, timestamp int64) *datura.Artifact {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole(channel)
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

func bookFrame(bidQty, askQty float64) string {
	return fmt.Sprintf(
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":%g}],"asks":[{"price":101.0,"qty":%g}]}]}`,
		bidQty, askQty,
	)
}

func loadedBookFrame() string {
	return `{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":20},{"price":99,"qty":18}],"asks":[{"price":101,"qty":8},{"price":102,"qty":6}]}]}`
}

func spoofBookFrame() string {
	return `{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":1},{"price":99,"qty":500},{"price":98,"qty":500}],"asks":[{"price":101,"qty":50},{"price":102,"qty":10}]}]}`
}

func tradeFrame(side string, quantity float64) string {
	return fmt.Sprintf(
		`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":%q,"price":100.5,"qty":%g,"timestamp":"2026-05-30T12:00:00Z"}]}`,
		side, quantity,
	)
}

var depthflowCategories = []logic.CategoryType{
	logic.CategoryLoadedImbalance,
	logic.CategorySpoofTrap,
	logic.CategoryBookThinning,
	logic.CategoryDenseNeutrality,
}

func categoryResult(result *datura.Artifact) int {
	return testutil.DominantCategoryIndex(result, depthflowCategories)
}

var depthflowClassifierInputs = []string{"loaded", "spoof", "thinning", "neutral"}

func outputScore(result *datura.Artifact, key string) float64 {
	return datura.Peek[float64](result, "output", key)
}

func winningClassifierInput(result *datura.Artifact) string {
	bestKey := depthflowClassifierInputs[0]
	bestScore := outputScore(result, bestKey)

	for _, key := range depthflowClassifierInputs[1:] {
		score := outputScore(result, key)

		if score > bestScore {
			bestScore = score
			bestKey = key
		}
	}

	return bestKey
}

func replayDepthflowBestScore(
	signal *Signal,
	frames []struct {
		channel string
		payload string
	},
	base int64,
	scoreKey string,
) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestScore      float64
		bestConfidence float64
	)

	for index, frame := range frames {
		datapoint := marketDatapoint(frame.channel, frame.payload, base+int64(index))
		measured := testutil.FirstMeasured(signal.Measure(datapoint, nil))

		if measured != nil {
			signal.tree = testutil.StoreMeasurement(signal.tree, measured)

			if scoreKey == "" {
				result = measured

				datapoint.Release()

				continue
			}

			if !testutil.HasConfidence(measured) {
				measured.Release()

				datapoint.Release()

				continue
			}

			score := outputScore(measured, scoreKey)
			confidence := datura.Peek[float64](measured, "output", "confidence")

			if winningClassifierInput(measured) != scoreKey {
				measured.Release()

				datapoint.Release()

				continue
			}

			if score > bestScore || (score == bestScore && confidence > bestConfidence) {
				if result != nil {
					result.Release()
				}

				result = measured
				bestScore = score
				bestConfidence = confidence
			}

			if result != measured {
				measured.Release()
			}
		}

		datapoint.Release()
	}

	return result
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given loaded bid-side depth with confirming buy pressure", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()
		frames := []struct {
			channel string
			payload string
		}{
			{"book", loadedBookFrame()},
			{"book", loadedBookFrame()},
			{"book", loadedBookFrame()},
			{"book", loadedBookFrame()},
			{"book", loadedBookFrame()},
			{"book", loadedBookFrame()},
			{"trade", tradeFrame("buy", 4)},
			{"trade", tradeFrame("buy", 4)},
			{"trade", tradeFrame("buy", 4)},
			{"book", loadedBookFrame()},
		}

		result := replayDepthflowBestScore(signal, frames, base, "loaded")

		Convey("It should classify loaded imbalance with loaded winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryLoadedImbalance))
			So(outputScore(result, "loaded"), ShouldBeGreaterThan, outputScore(result, "spoof"))
			So(winningClassifierInput(result), ShouldEqual, "loaded")
			So(outputScore(result, "confidence"), ShouldBeGreaterThan, 0.25)
		})
	})

	Convey("Given heavy bid depth contradicted by sell trades", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC).UnixNano()
		frames := []struct {
			channel string
			payload string
		}{
			{"book", loadedBookFrame()},
			{"book", loadedBookFrame()},
			{"book", loadedBookFrame()},
			{"book", loadedBookFrame()},
			{"book", loadedBookFrame()},
			{"book", loadedBookFrame()},
			{"book", spoofBookFrame()},
			{"trade", tradeFrame("sell", 8)},
			{"trade", tradeFrame("sell", 8)},
			{"trade", tradeFrame("sell", 8)},
			{"book", spoofBookFrame()},
		}

		result := replayDepthflowBestScore(signal, frames, base, "spoof")

		Convey("It should classify spoof trap with spoof winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategorySpoofTrap))
			So(outputScore(result, "spoof"), ShouldBeGreaterThan, outputScore(result, "loaded"))
			So(winningClassifierInput(result), ShouldEqual, "spoof")
			So(outputScore(result, "confidence"), ShouldBeGreaterThan, 0.25)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		for index := range 8 {
			datapoint := marketDatapoint("book", bookFrame(20-float64(index), 10), base+int64(index))
			_ = testutil.FirstMeasured(signal.Measure(datapoint, nil))
			datapoint.Release()
		}

		_ = signal.Close()
	}
}
