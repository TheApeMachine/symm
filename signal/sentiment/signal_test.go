package sentiment

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

func tickerDatapoint(symbol string, last, changePct float64, timestamp int64) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":%g,"volume":1000,"change_pct":%g}]}`,
		symbol, last, changePct,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given broad positive market breadth with a leading symbol", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}
		var result *datura.Artifact

		for tick := range 20 {
			at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

			for symbolIndex, symbol := range symbols {
				changePct := 1.0 + float64(tick)*0.05 + float64(symbolIndex)*0.1
				last := 100 + float64(tick) + float64(symbolIndex)
				datapoint := tickerDatapoint(symbol, last, changePct, at)
				measured := signal.Measure(datapoint)

				if measured != nil {
					result = measured
				}

				datapoint.Release()
			}
		}

		Convey("It should emit risk-on surge only with leadership confirmation", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 1.0/3.0)
			So(int(datura.Peek[float64](result, "output", "category")), ShouldEqual, 1)
		})
	})

	Convey("Given broad positive market breadth without leadership on the symbol", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		leaders := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
		var laggardResult *datura.Artifact

		for tick := range 20 {
			at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

			for symbolIndex, symbol := range leaders {
				changePct := 2.0 + float64(tick)*0.05 + float64(symbolIndex)*0.2
				last := 100 + float64(tick) + float64(symbolIndex)
				datapoint := tickerDatapoint(symbol, last, changePct, at)
				_ = signal.Measure(datapoint)
				datapoint.Release()
			}

			laggard := tickerDatapoint("FLAT/USD", 100, 0.01, at)
			measured := signal.Measure(laggard)

			if measured != nil {
				laggardResult = measured
			}

			laggard.Release()
		}

		Convey("It should not classify risk-on surge without leader status", func() {
			So(laggardResult, ShouldNotBeNil)
			So(int(datura.Peek[float64](laggardResult, "output", "category")), ShouldNotEqual, 1)
		})
	})

	Convey("Given flat market breadth with zero majority threshold", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		first := tickerDatapoint("BTC/USD", 100, 0, base.UnixNano())
		_ = signal.Measure(first)
		first.Release()

		second := tickerDatapoint("ETH/USD", 100, 0, base.Add(time.Minute).UnixNano())
		result := signal.Measure(second)
		second.Release()

		Convey("It should defer conviction until breadth threshold is positive", func() {
			So(result, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		for tick := range 20 {
			at := base.Add(time.Duration(tick) * time.Minute).UnixNano()

			for symbolIndex, symbol := range symbols {
				datapoint := tickerDatapoint(symbol, 100+float64(tick), 1+float64(symbolIndex), at)
				_ = signal.Measure(datapoint)
				datapoint.Release()
			}
		}

		_ = signal.Close()
	}
}
