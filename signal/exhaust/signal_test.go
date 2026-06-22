package exhaust

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
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		bidDepths := []float64{20, 18, 14, 10, 6, 3, 1.5, 0.5}
		var result *datura.Artifact

		for index, bidQty := range bidDepths {
			datapoint := bookDatapoint(bidQty, 10, base.Add(time.Duration(index)*time.Second).UnixNano())
			measured := signal.Measure(datapoint)

			if measured != nil {
				if result != nil {
					result.Release()
				}

				result = measured
			}

			datapoint.Release()
		}

		Convey("It should classify mechanical collapse with mechanical winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "mechanical"), ShouldBeGreaterThan, outputScore(result, "thermal"))
			So(outputScore(result, "mechanical"), ShouldBeGreaterThan, outputScore(result, "fragile"))
			So(winningClassifierInput(result), ShouldEqual, "mechanical")
			So(categoryResult(result), ShouldEqual, 1)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)

			result.Release()
		})
	})

	Convey("Given fading aggressive buy pressure", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		var result *datura.Artifact

		for index := range 12 {
			at := base.Add(time.Duration(index) * time.Second).UnixNano()
			bookFrame := bookDatapoint(10, 10, at)
			_ = signal.Measure(bookFrame)
			bookFrame.Release()
		}

		tradeSizes := []float64{20, 18, 16, 14, 12, 10, 8, 6, 4, 2, 1, 0.5}

		for index, quantity := range tradeSizes {
			at := base.Add(time.Duration(index+12) * time.Second).UnixNano()
			tradeFrame := tradeDatapoint("buy", 100, quantity, at)
			_ = signal.Measure(tradeFrame)
			tradeFrame.Release()

			bookFrame := bookDatapoint(10, 10, at+1)
			measured := signal.Measure(bookFrame)

			if measured != nil {
				if result != nil {
					result.Release()
				}

				result = measured
			}

			bookFrame.Release()
		}

		Convey("It should classify thermal exhaustion with thermal winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, 3)
			So(outputScore(result, "thermal"), ShouldBeGreaterThan, outputScore(result, "mechanical"))
			So(winningClassifierInput(result), ShouldEqual, "thermal")

			result.Release()
		})
	})

	Convey("Given stable book depth after warmup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()
		var lastResult *datura.Artifact

		for index := range 12 {
			datapoint := bookDatapoint(10, 10, base+int64(index))
			lastResult = signal.Measure(datapoint)
			datapoint.Release()
		}

		Convey("It should not emit a measurement on zero decay urgency", func() {
			So(lastResult, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		for index := range 8 {
			datapoint := bookDatapoint(20-float64(index)*2, 10, base+int64(index))
			_ = signal.Measure(datapoint)
			datapoint.Release()
		}

		_ = signal.Close()
	}
}
