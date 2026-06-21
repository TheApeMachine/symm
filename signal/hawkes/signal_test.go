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

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given a warmed trade burst", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		var result *datura.Artifact

		for index := range 128 {
			side := "buy"

			if index%2 == 0 {
				side = "sell"
			}

			at := base.Add(time.Duration(index) * 100 * time.Millisecond).UnixNano()
			frame := tradeDatapoint("ALT/EUR", side, 1, 1, at)
			measured := signal.Measure(frame)

			if measured != nil {
				result = measured
			}

			frame.Release()
		}

		Convey("It should emit calibrated thermal classification", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)
			So(datura.Peek[float64](result, "output", "value"), ShouldBeGreaterThan, 0)
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
