package correlation

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
	Convey("Given correlated cross-section returns", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
		var result *datura.Artifact

		for tick := range 8 {
			at := base.Add(time.Duration(tick) * 10 * time.Second).UnixNano()
			changePct := 0.5 + float64(tick)*0.1

			for symbolIndex, symbol := range symbols {
				last := 100 + float64(tick) + float64(symbolIndex)*0.01
				datapoint := tickerDatapoint(symbol, last, changePct, at)
				measured := signal.Measure(datapoint)

				if measured != nil {
					result = measured
				}

				datapoint.Release()
			}
		}

		Convey("It should emit non-uniform cohort classification", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)

			category := int(datura.Peek[float64](result, "output", "category"))
			scoreKeys := []string{"herdScore", "alphaScore", "noiseScore", "stressScore"}

			So(category, ShouldBeBetweenOrEqual, 1, len(scoreKeys))
			So(datura.Peek[float64](result, "output", scoreKeys[category-1]), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		for tick := range 8 {
			at := base.Add(time.Duration(tick) * 10 * time.Second).UnixNano()

			for symbolIndex, symbol := range symbols {
				datapoint := tickerDatapoint(symbol, 100+float64(tick)+float64(symbolIndex), 0.5, at)
				_ = signal.Measure(datapoint)
				datapoint.Release()
			}
		}

		_ = signal.Close()
	}
}
