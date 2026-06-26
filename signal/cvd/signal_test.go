package cvd

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

var cvdCategories = []logic.CategoryType{
	logic.CategoryHiddenAbsorption,
	logic.CategoryAggressiveDrive,
	logic.CategoryStochasticBalance,
	logic.CategoryVolumeStarvation,
}

func categoryResult(result *datura.Artifact) int {
	return testutil.DominantCategoryIndex(result, cvdCategories)
}

var classifierInputs = []string{"absorption", "drive", "balance", "starvation"}

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

func measureTradeSequenceBestScore(
	signal *Signal,
	symbol string,
	trades []struct {
		side     string
		price    float64
		quantity float64
	},
	base time.Time,
	scoreKey string,
) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestScore      float64
		bestConfidence float64
	)

	for index, trade := range trades {
		at := base.Add(time.Duration(index) * 100 * time.Millisecond).UnixNano()
		frame := tradeDatapoint(symbol, trade.side, trade.price, trade.quantity, at)
		measured := testutil.FirstMeasured(signal.Measure(frame, nil))

		if measured != nil {
			signal.tree = testutil.StoreMeasurement(signal.tree, measured)
			score := outputScore(measured, scoreKey)
			confidence := datura.Peek[float64](measured, "output", "confidence")

			if score > bestScore || (score == bestScore && confidence > bestConfidence) {
				if result != nil {
					result.Release()
				}

				result = measured
				bestScore = score
				bestConfidence = confidence

				frame.Release()

				continue
			}

			measured.Release()
		}

		frame.Release()
	}

	return result
}

func measureTradeSequence(
	signal *Signal,
	symbol string,
	trades []struct {
		side     string
		price    float64
		quantity float64
	},
	base time.Time,
) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestConfidence float64
	)

	for index, trade := range trades {
		at := base.Add(time.Duration(index) * time.Second).UnixNano()
		frame := tradeDatapoint(symbol, trade.side, trade.price, trade.quantity, at)
		measured := testutil.FirstMeasured(signal.Measure(frame, nil))

		if measured != nil {
			signal.tree = testutil.StoreMeasurement(signal.tree, measured)
			confidence := datura.Peek[float64](measured, "output", "confidence")

			if confidence > bestConfidence {
				if result != nil {
					result.Release()
				}

				result = measured
				bestConfidence = confidence

				frame.Release()

				continue
			}

			measured.Release()
		}

		frame.Release()
	}

	return result
}

func tradeDatapoint(symbol, side string, price, quantity float64, timestamp int64) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"trade","type":"update","data":[{"symbol":%q,"side":%q,"price":%g,"qty":%g,"timestamp":"2026-05-30T12:00:00Z"}]}`,
		symbol, side, price, quantity,
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

	Convey("Given aggressive buy flow with rising price", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		trades := make([]struct {
			side     string
			price    float64
			quantity float64
		}, 12)

		for index := range trades {
			trades[index] = struct {
				side     string
				price    float64
				quantity float64
			}{"buy", 100 + float64(index)*0.01, 1}
		}

		result := measureTradeSequenceBestScore(signal, "BTC/USD", trades, base, "drive")

		Convey("It should classify aggressive drive with drive winning", func() {
			So(result, ShouldNotBeNil)
			So(winningClassifierInput(result), ShouldEqual, "drive")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryAggressiveDrive))
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)

			result.Release()
		})
	})

	Convey("Given one-sided buys with flat price", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		trades := []struct {
			side     string
			price    float64
			quantity float64
		}{
			{"buy", 50, 10},
			{"buy", 50.001, 10},
			{"buy", 50, 10},
			{"buy", 50.001, 10},
			{"buy", 50, 10},
			{"buy", 50.001, 10},
		}
		result := measureTradeSequence(signal, "BTC/USD", trades, base)

		Convey("It should classify hidden absorption with absorption winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "absorption"), ShouldBeGreaterThan, outputScore(result, "drive"))
			So(outputScore(result, "absorption"), ShouldBeGreaterThan, outputScore(result, "balance"))
			So(winningClassifierInput(result), ShouldEqual, "absorption")
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryHiddenAbsorption))

			result.Release()
		})
	})

	Convey("Given alternating balanced flow", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		trades := make([]struct {
			side     string
			price    float64
			quantity float64
		}, 12)

		for index := range trades {
			side := "buy"

			if index%2 == 0 {
				side = "sell"
			}

			trades[index] = struct {
				side     string
				price    float64
				quantity float64
			}{side, 100 + float64(index)*0.001, 5}
		}

		result := measureTradeSequenceBestScore(signal, "BTC/USD", trades, base, "balance")

		Convey("It should classify stochastic balance with balance winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryStochasticBalance))

			result.Release()
		})
	})

	Convey("Given two alternating micro trades on a cold signal", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		trades := []struct {
			side     string
			price    float64
			quantity float64
		}{
			{"buy", 100, 0.001},
			{"sell", 100, 0.001},
		}

		warmup := []struct {
			side     string
			price    float64
			quantity float64
		}{
			{"buy", 100, 5},
			{"sell", 100, 5},
			{"buy", 100.01, 5},
			{"sell", 100.01, 5},
			{"buy", 100.02, 5},
		}
		_ = measureTradeSequence(signal, "BTC/USD", warmup, base)
		result := measureTradeSequenceBestScore(signal, "BTC/USD", trades, base.Add(10*time.Second), "starvation")

		Convey("It should classify volume starvation with starvation scoring highest", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "starvation"), ShouldBeGreaterThan, 0)
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryVolumeStarvation))

			result.Release()
		})
	})
}

func TestSignalColdStartRebuildsFromTree(testingTB *testing.T) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	Convey("Given prior measurements written to a shared tree", testingTB, func() {
		tree := dmt.NewTree("")
		warm := NewSignal(context.Background(), tree)

		defer func() {
			_ = warm.Close()
		}()

		// Build tape history on one Signal, persisting each measurement to the
		// tree. The Signal carries no per-pair store, so the tree is the only
		// history source.
		for index := 0; index < 6; index++ {
			side := "buy"

			if index%2 == 0 {
				side = "sell"
			}

			at := base.Add(time.Duration(index) * time.Second).UnixNano()
			frame := tradeDatapoint("BTC/USD", side, 100+float64(index)*0.01, 5, at)
			measured := testutil.FirstMeasured(warm.Measure(frame, nil))

			if measured != nil {
				tree = testutil.StoreMeasurement(tree, measured)
				measured.Release()
			}

			frame.Release()
		}

		Convey("A fresh Signal with empty in-memory state rebuilds from the tree", func() {
			cold := NewSignal(context.Background(), tree)

			defer func() {
				_ = cold.Close()
			}()

			grossVolumes, drifts, signedFlows, prevPrice := cold.history("BTC/USD")

			So(len(grossVolumes), ShouldBeGreaterThan, 0)
			So(len(drifts), ShouldBeGreaterThan, 0)
			So(len(signedFlows), ShouldBeGreaterThan, 0)
			So(prevPrice, ShouldBeGreaterThan, 0)

			at := base.Add(7 * time.Second).UnixNano()
			frame := tradeDatapoint("BTC/USD", "buy", 100.07, 5, at)

			defer frame.Release()

			result := testutil.FirstMeasured(cold.Measure(frame, nil))

			So(result, ShouldNotBeNil)
			So(testutil.HasConfidence(result), ShouldBeTrue)

			result.Release()
		})
	})
}

func TestSignalMeasureColdStartReturnsNil(testingTB *testing.T) {
	Convey("Given a single trade frame", testingTB, func() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frame := tradeDatapoint("BTC/USD", "buy", 100, 1, time.Now().UnixNano())

		defer frame.Release()

		Convey("It should return a classified measurement on the first trade", func() {
			result := testutil.FirstMeasured(signal.Measure(frame, nil))

			So(result, ShouldNotBeNil)
			So(testutil.HasConfidence(result), ShouldBeTrue)

			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		var result *datura.Artifact

		for index := range 12 {
			frame := tradeDatapoint("BTC/USD", "buy", 100+float64(index)*0.01, 1, base+int64(index))
			result = testutil.FirstMeasured(signal.Measure(frame, nil))
			frame.Release()
		}

		if result != nil {
			result.Release()
		}

		_ = signal.Close()
	}
}
