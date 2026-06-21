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

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given crumbling bid-side depth", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()
		bidDepths := []float64{20, 18, 14, 10, 6, 3, 1.5, 0.5}
		var result *datura.Artifact

		for index, bidQty := range bidDepths {
			datapoint := bookDatapoint(bidQty, 10, base+int64(index))
			measured := signal.Measure(datapoint)

			if measured != nil {
				result = measured
			}

			datapoint.Release()
		}

		Convey("It should emit non-uniform decay classification", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)
			So(datura.Peek[float64](result, "output", "mechanical"), ShouldBeGreaterThan, 0)
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
