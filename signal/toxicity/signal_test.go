package toxicity

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func categoryResult(result *datura.Artifact) int {
	return int(datura.Peek[float64](result, "output", "category"))
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

func classifierEvidence(result *datura.Artifact) float64 {
	return outputScore(result, "bluffScore") +
		outputScore(result, "vacuumScore") +
		outputScore(result, "supportScore")
}

func measureBookFrames(signal *Signal, frames []string) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestConfidence float64
		bestEvidence   float64
	)

	for _, frame := range frames {
		datapoint := bookDatapoint(frame)
		measured := signal.Measure(datapoint)

		if measured != nil {
			confidence := datura.Peek[float64](measured, "output", "confidence")
			evidence := classifierEvidence(measured)

			if evidence > bestEvidence ||
				(evidence == bestEvidence && confidence > bestConfidence) {
				if result != nil {
					result.Release()
				}

				result = measured
				bestConfidence = confidence
				bestEvidence = evidence

				datapoint.Release()

				continue
			}

			measured.Release()
		}

		datapoint.Release()
	}

	return result
}

func measureBookFramesForScore(signal *Signal, frames []string, scoreKey string) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestScore      float64
		bestConfidence float64
	)

	for _, frame := range frames {
		datapoint := bookDatapoint(frame)
		measured := signal.Measure(datapoint)

		if measured != nil {
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

func bookDatapoint(payload string) *datura.Artifact {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("book")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))

	return artifact
}

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
	frames := bookWarmupFrames(80, 80, 15)

	churnFrames := []string{
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":80.0},{"price":100.0,"qty":40.0}],"asks":[{"price":101.0,"qty":80.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":80.0},{"price":100.0,"qty":20.0}],"asks":[{"price":101.0,"qty":80.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":80.0},{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":80.0}]}]}`,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":80.0},{"price":100.0,"qty":5.0}],"asks":[{"price":101.0,"qty":80.0}]}]}`,
	}

	frames = append(frames, churnFrames...)
	frames = append(frames,
		bookFrame(80, 80),
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":80.0},{"price":100.0,"qty":2.0}],"asks":[{"price":101.0,"qty":80.0}]}]}`,
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
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		frames := bookQualityBluffFrames()
		result := measureBookFramesForScore(signal, frames, "bluffScore")

		Convey("It should classify toxic bluff with bluffScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "bluffScore"), ShouldBeGreaterThan, 0)
			So(outputScore(result, "bluffScore"), ShouldBeGreaterThan, outputScore(result, "vacuumScore"))
			So(outputScore(result, "bluffScore"), ShouldBeGreaterThan, outputScore(result, "supportScore"))
			So(winningClassifierInput(result), ShouldEqual, "bluffScore")
			So(categoryResult(result), ShouldEqual, 1)

			result.Release()
		})
	})

	Convey("Given a warmed ask-side liquidity vacuum", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		frames := bookQualityVacuumFrames()
		result := measureBookFramesForScore(signal, frames, "vacuumScore")

		Convey("It should classify liquidity vacuum with vacuumScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "vacuumScore"), ShouldBeGreaterThan, 0)
			So(outputScore(result, "vacuumScore"), ShouldBeGreaterThan, outputScore(result, "bluffScore"))
			So(outputScore(result, "vacuumScore"), ShouldBeGreaterThan, outputScore(result, "supportScore"))
			So(winningClassifierInput(result), ShouldEqual, "vacuumScore")
			So(categoryResult(result), ShouldEqual, 2)

			result.Release()
		})
	})

	Convey("Given a warmed sincere support book", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		frames := bookQualitySupportFrames()
		result := measureBookFrames(signal, frames)

		Convey("It should classify hard support with supportScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "supportScore"), ShouldBeGreaterThan, outputScore(result, "bluffScore"))
			So(outputScore(result, "supportScore"), ShouldBeGreaterThan, outputScore(result, "vacuumScore"))
			So(winningClassifierInput(result), ShouldEqual, "supportScore")
			So(categoryResult(result), ShouldEqual, 3)

			result.Release()
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a warmed book replay", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
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
			measured := signal.Measure(datapoint)

			if measured != nil {
				result = measured

				confidence := datura.Peek[float64](result, "output", "confidence")

				if confidence > bestConfidence {
					bestConfidence = confidence
				}
			}

			datapoint.Release()
		}

		Convey("It returns classifier output with non-uniform confidence", func() {
			So(result, ShouldNotBeNil)

			role, _ := result.Role()
			scope, _ := result.Scope()

			So(role, ShouldEqual, "book")
			So(scope, ShouldEqual, "update")
			So(datura.Peek[string](result, "channel"), ShouldEqual, "book")
			So(len(result.DecryptPayload()), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
			So(bestConfidence, ShouldNotAlmostEqual, 1.0/3.0, 0.0001)

			result.Release()
		})
	})

	Convey("Given a single book frame without warmup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		datapoint := bookDatapoint(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`)

		defer datapoint.Release()

		result := signal.Measure(datapoint)

		Convey("It should not emit an uncalibrated measurement", func() {
			So(result, ShouldBeNil)
		})
	})
}

func TestMeasureBookFrames(testingTB *testing.T) {
	Convey("Given live-shaped kraken book fixtures", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		signal := NewSignal(ctx, newTestPool(testingTB), dmt.NewTree(""))

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
			measured := signal.Measure(datapoint)

			if measured != nil {
				result = measured
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
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		var result *datura.Artifact

		for _, frame := range frames {
			datapoint := bookDatapoint(frame)
			measured := signal.Measure(datapoint)

			if measured != nil {
				result = measured
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
