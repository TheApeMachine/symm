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
	Convey("Given aggressive buy flow with rising price", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()
		var result *datura.Artifact

		for index := range 12 {
			frame := tradeDatapoint("BTC/USD", "buy", 100+float64(index)*0.01, 1, base+int64(index))
			measured := signal.Measure(frame)

			if measured != nil {
				result = measured
			}

			frame.Release()
		}

		Convey("It should classify aggressive drive", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "drive"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "drive"), ShouldBeGreaterThan,
				datura.Peek[float64](result, "output", "absorption"))
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)
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
