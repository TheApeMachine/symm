package cvd

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func categoryResult(result *datura.Artifact) int {
	return int(datura.Peek[float64](result, "output", "category"))
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
		at := base.Add(time.Duration(index) * time.Second).UnixNano()
		frame := tradeDatapoint(symbol, trade.side, trade.price, trade.quantity, at)
		measured := signal.Measure(frame)

		if measured != nil {
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
		measured := signal.Measure(frame)

		if measured != nil {
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
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
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

		result := measureTradeSequence(signal, "BTC/USD", trades, base)

		Convey("It should classify aggressive drive with drive winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "drive"), ShouldBeGreaterThan, outputScore(result, "absorption"))
			So(outputScore(result, "drive"), ShouldBeGreaterThan, outputScore(result, "balance"))
			So(winningClassifierInput(result), ShouldEqual, "drive")
			So(categoryResult(result), ShouldEqual, 2)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)

			result.Release()
		})
	})

	Convey("Given one-sided buys with flat price", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
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
			So(categoryResult(result), ShouldEqual, 1)

			result.Release()
		})
	})

	Convey("Given alternating balanced flow", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
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

		result := measureTradeSequence(signal, "BTC/USD", trades, base)

		Convey("It should classify stochastic balance with balance winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "balance"), ShouldBeGreaterThan, outputScore(result, "drive"))
			So(outputScore(result, "balance"), ShouldBeGreaterThan, outputScore(result, "absorption"))
			So(winningClassifierInput(result), ShouldEqual, "balance")
			So(categoryResult(result), ShouldEqual, 3)

			result.Release()
		})
	})

	Convey("Given tiny trade quantities after warmup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		warmup := make([]struct {
			side     string
			price    float64
			quantity float64
		}, 12)

		for index := range warmup {
			warmup[index] = struct {
				side     string
				price    float64
				quantity float64
			}{"buy", 100, 1}
		}

		starvation := []struct {
			side     string
			price    float64
			quantity float64
		}{}

		for range 48 {
			starvation = append(starvation, struct {
				side     string
				price    float64
				quantity float64
			}{"buy", 100, 0.001})
		}

		trades := append(warmup, starvation...)
		result := measureTradeSequenceBestScore(signal, "BTC/USD", trades, base, "starvation")

		Convey("It should classify volume starvation with starvation scoring highest", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 4)
			So(outputScore(result, "starvation"), ShouldBeGreaterThan, outputScore(result, "drive"))
			So(outputScore(result, "starvation"), ShouldBeGreaterThan, outputScore(result, "absorption"))
			So(outputScore(result, "starvation"), ShouldBeGreaterThan, 0)

			result.Release()
		})
	})
}

func TestSignalMeasureColdStartReturnsNil(testingTB *testing.T) {
	Convey("Given a single trade frame", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		frame := tradeDatapoint("BTC/USD", "buy", 100, 1, time.Now().UnixNano())

		defer frame.Release()

		Convey("It should not emit an uncalibrated measurement", func() {
			So(signal.Measure(frame), ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		var result *datura.Artifact

		for index := range 12 {
			frame := tradeDatapoint("BTC/USD", "buy", 100+float64(index)*0.01, 1, base+int64(index))
			result = signal.Measure(frame)
			frame.Release()
		}

		if result != nil {
			result.Release()
		}

		_ = signal.Close()
	}
}
