package hawkes

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

func bookDatapoint(bidQty, askQty float64, timestamp int64) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"book","type":"update","data":[{"symbol":"ALT/EUR","bids":[{"price":100.0,"qty":%g}],"asks":[{"price":101.0,"qty":%g}]}]}`,
		bidQty, askQty,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("book")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

func categoryResult(result *datura.Artifact) int {
	return int(datura.Peek[float64](result, "output", "category"))
}

var hawkesClassifierInputs = []string{"frenzy", "saturation", "organic", "exhaustion"}

func outputScore(result *datura.Artifact, key string) float64 {
	return datura.Peek[float64](result, "output", key)
}

func winningClassifierInput(result *datura.Artifact) string {
	bestKey := hawkesClassifierInputs[0]
	bestScore := outputScore(result, bestKey)

	for _, key := range hawkesClassifierInputs[1:] {
		score := outputScore(result, key)

		if score > bestScore {
			bestScore = score
			bestKey = key
		}
	}

	return bestKey
}

func measureBuyBurstWithBook(
	signal *Signal,
	burstStart time.Time,
	tradeCount int,
	interval time.Duration,
) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestConfidence float64
	)

	for index := range tradeCount {
		at := burstStart.Add(time.Duration(index) * interval).UnixNano()
		book := bookDatapoint(50, 3, at)
		_ = signal.Measure(book)
		book.Release()

		frame := tradeDatapoint("ALT/EUR", "buy", 1+float64(index)*0.001, 4, at)

		for range 4 {
			measured := signal.Measure(frame)

			if measured == nil {
				continue
			}

			confidence := datura.Peek[float64](measured, "output", "confidence")

			if confidence > bestConfidence {
				if result != nil {
					result.Release()
				}

				result = measured
				bestConfidence = confidence
				continue
			}

			measured.Release()
		}

		frame.Release()
	}

	return result
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given a one-sided buy burst with bid-side book support", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		for index := range 96 {
			at := base.Add(time.Duration(index) * 500 * time.Millisecond).UnixNano()
			side := "buy"

			if index%5 == 0 {
				side = "sell"
			}

			frame := tradeDatapoint("ALT/EUR", side, 1, 1, at)
			_ = signal.Measure(frame)
			frame.Release()
		}

		burstStart := base.Add(96 * 500 * time.Millisecond)
		result := measureBuyBurstWithBook(signal, burstStart, 160, 200*time.Millisecond)

		Convey("It should classify frenzy with the frenzy score winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 1)
			So(outputScore(result, "frenzy"), ShouldBeGreaterThan, outputScore(result, "saturation"))
			So(outputScore(result, "frenzy"), ShouldBeGreaterThan, outputScore(result, "organic"))
			So(winningClassifierInput(result), ShouldEqual, "frenzy")
			So(outputScore(result, "confidence"), ShouldBeGreaterThan, 0.25)

			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		for index := range 32 {
			side := "buy"

			if index%2 == 0 {
				side = "sell"
			}

			at := base.Add(time.Duration(index) * 100 * time.Millisecond).UnixNano()
			frame := tradeDatapoint("ALT/EUR", side, 1, 1, at)
			_ = signal.Measure(frame)
			frame.Release()
		}

		_ = signal.Close()
	}
}
