package toxicity

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

var toxicityCategories = []logic.CategoryType{
	logic.CategoryToxicBluff,
	logic.CategoryLiquidityVacuum,
	logic.CategoryHardSupport,
}

func categoryResult(result *datura.Artifact) int {
	return testutil.DominantCategoryIndex(result, toxicityCategories)
}

var classifierInputs = []string{"bluffScore", "vacuumScore", "supportScore"}

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

func bookFrame(bidQty, askQty float64) string {
	return fmt.Sprintf(
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":%g}],"asks":[{"price":101.0,"qty":%g}]}]}`,
		bidQty, askQty,
	)
}

func bookWarmupFrames(bidQty, askQty float64, count int) []string {
	frame := bookFrame(bidQty, askQty)
	frames := make([]string, count)

	for index := range frames {
		frames[index] = frame
	}

	return frames
}

func measureBookFramesForScore(signal *Signal, frames []string, scoreKey string) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestScore      float64
		bestConfidence float64
	)

	for _, frame := range frames {
		datapoint := bookDatapoint(frame)
		measured := signal.Measure(datapoint, nil)

		if measured != nil {
			signal.tree = testutil.StoreMeasurement(signal.tree, measured)
			score := outputScore(measured, scoreKey)
			confidence := datura.Peek[float64](measured, "output", "confidence")

			if score <= 0 {
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

				datapoint.Release()

				continue
			}

			measured.Release()
		}

		datapoint.Release()
	}

	return result
}

func bookDatapoint(payload string) *datura.Artifact {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("book")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	bookReplaySequence += int64(time.Millisecond)
	artifact.SetTimestamp(bookReplaySequence)

	return artifact
}

var bookReplaySequence = time.Now().UnixNano()

const bookQualityWarmupFrames = 3

func bookQualityVacuumFrames() []string {
	return []string{
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":10}],"asks":[{"price":101,"qty":10}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":10}],"asks":[{"price":101,"qty":10}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":10}],"asks":[{"price":101,"qty":10}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":12}],"asks":[{"price":101,"qty":10}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":12}],"asks":[{"price":101,"qty":10}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":12}],"asks":[{"price":101,"qty":10}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":10}],"asks":[{"price":101,"qty":10}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":10}],"asks":[{"price":101,"qty":10}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":3}],"asks":[{"price":101,"qty":10}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":1}],"asks":[{"price":101,"qty":10}]}]}`,
	}
}

func bookQualityBluffFrames() []string {
	frames := bookWarmupFrames(50, 80, 12)

	for range 8 {
		frames = append(frames, bookFrame(80, 80), bookFrame(62, 62))
	}

	frames = append(frames,
		bookFrame(110, 110),
		bookFrame(45, 45),
		bookFrame(110, 110),
		bookFrame(45, 45),
		bookFrame(105, 105),
		bookFrame(40, 40),
	)

	return frames
}

func bookQualitySupportFrames() []string {
	frames := bookWarmupFrames(10, 10, bookQualityWarmupFrames)
	frames = append(frames,
		bookFrame(10, 10),
		bookFrame(12, 12),
		bookFrame(14, 14),
		bookFrame(16, 16),
		bookFrame(18, 18),
		bookFrame(20, 20),
	)

	return frames
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given a warmed toxic bluff at the touch", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		frames := bookQualityBluffFrames()
		result := measureBookFramesForScore(signal, frames, "bluffScore")

		Convey("It should classify toxic bluff with bluffScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "bluffScore"), ShouldBeGreaterThan, outputScore(result, "vacuumScore"))
			So(outputScore(result, "bluffScore"), ShouldBeGreaterThan, outputScore(result, "supportScore"))
			So(winningClassifierInput(result), ShouldEqual, "bluffScore")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryToxicBluff))

			result.Release()
		})
	})

	Convey("Given a warmed ask-side liquidity vacuum", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		frames := bookQualityVacuumFrames()
		result := measureBookFramesForScore(signal, frames, "vacuumScore")

		Convey("It should classify liquidity vacuum with vacuumScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "vacuumScore"), ShouldBeGreaterThan, outputScore(result, "bluffScore"))
			So(outputScore(result, "vacuumScore"), ShouldBeGreaterThan, outputScore(result, "supportScore"))
			So(winningClassifierInput(result), ShouldEqual, "vacuumScore")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryLiquidityVacuum))

			result.Release()
		})
	})

	Convey("Given a warmed sincere support book", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		frames := bookQualitySupportFrames()
		result := measureBookFramesForScore(signal, frames, "supportScore")

		Convey("It should classify hard support with supportScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "supportScore"), ShouldBeGreaterThan, outputScore(result, "bluffScore"))
			So(outputScore(result, "supportScore"), ShouldBeGreaterThan, outputScore(result, "vacuumScore"))
			So(winningClassifierInput(result), ShouldEqual, "supportScore")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryHardSupport))

			result.Release()
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a warmed book replay", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		frames := []string{
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":3.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":1.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		}

		var (
			result         *datura.Artifact
			bestConfidence float64
		)

		for _, frame := range frames {
			datapoint := bookDatapoint(frame)
			measured := signal.Measure(datapoint, nil)

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)

				if !testutil.HasConfidence(measured) {
					measured.Release()

					datapoint.Release()

					continue
				}

				confidence := datura.Peek[float64](measured, "output", "confidence")

				if confidence > bestConfidence {
					result = measured
					bestConfidence = confidence
				}
			}

			datapoint.Release()
		}

		Convey("It returns classifier output with non-uniform confidence", func() {
			So(result, ShouldNotBeNil)

			role, _ := result.Role()
			scope, _ := result.Scope()

			So(role, ShouldEqual, "measurement")
			So(scope, ShouldEqual, "BTC/USD")
			So(len(result.DecryptPayload()), ShouldBeGreaterThan, 0)
			So(testutil.DistributionSum(result, toxicityCategories), ShouldAlmostEqual, 1, 0.0001)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
			So(bestConfidence, ShouldNotAlmostEqual, 1.0/3.0, 0.0001)

			result.Release()
		})
	})

	Convey("Given a single book frame without warmup", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		datapoint := bookDatapoint(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`)

		defer datapoint.Release()

		result := signal.Measure(datapoint, nil)

		Convey("It should publish on the first book observation", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestMeasureBookFrames(testingTB *testing.T) {
	Convey("Given live-shaped kraken book fixtures", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		signal := NewSignal(ctx, dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		var result *datura.Artifact

		frames := []string{
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":3.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":1.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		}

		for _, frame := range frames {
			datapoint := bookDatapoint(frame)
			measured := signal.Measure(datapoint, nil)

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)

				if testutil.HasConfidence(measured) {
					result = measured
				}
			}

			datapoint.Release()
		}

		Convey("It should emit classifier output on a writable measurement artifact", func() {
			So(result, ShouldNotBeNil)
			So(len(result.DecryptPayload()), ShouldBeGreaterThan, 2)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)

			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	frames := []string{
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":12.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":3.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
	}

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		var result *datura.Artifact

		for _, frame := range frames {
			datapoint := bookDatapoint(frame)
			measured := signal.Measure(datapoint, nil)

			if measured != nil {
				signal.tree = testutil.StoreMeasurement(signal.tree, measured)

				if testutil.HasConfidence(measured) {
					result = measured
				}
			}

			datapoint.Release()
		}

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		result.Release()
		_ = signal.Close()
	}
}
