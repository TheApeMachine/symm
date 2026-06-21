package depthflow

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

func tradeFrame(side string, quantity float64) string {
	return fmt.Sprintf(
		`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":%q,"price":100.5,"qty":%g,"timestamp":"2026-05-30T12:00:00Z"}]}`,
		side, quantity,
	)
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given loaded bid-side depth with confirming buy pressure", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()
		frames := []struct {
			channel string
			payload string
		}{
			{"book", bookFrame(20, 10)},
			{"book", bookFrame(20, 10)},
			{"book", bookFrame(20, 10)},
			{"book", bookFrame(18, 10)},
			{"trade", tradeFrame("buy", 2)},
			{"trade", tradeFrame("buy", 2)},
			{"book", bookFrame(16, 10)},
			{"trade", tradeFrame("buy", 3)},
		}

		var result *datura.Artifact

		for index, frame := range frames {
			datapoint := marketDatapoint(frame.channel, frame.payload, base+int64(index))
			measured := signal.Measure(datapoint)

			if measured != nil {
				result = measured
			}

			datapoint.Release()
		}

		Convey("It should emit non-uniform depth-flow classification", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)
			So(
				datura.Peek[float64](result, "output", "loadedScore")+
					datura.Peek[float64](result, "output", "spoofScore")+
					datura.Peek[float64](result, "output", "thinScore")+
					datura.Peek[float64](result, "output", "neutralScore"),
				ShouldBeGreaterThan,
				0,
			)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		for index := range 8 {
			datapoint := marketDatapoint("book", bookFrame(20-float64(index), 10), base+int64(index))
			_ = signal.Measure(datapoint)
			datapoint.Release()
		}

		_ = signal.Close()
	}
}
