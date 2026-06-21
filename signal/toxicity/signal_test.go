package toxicity

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/tests"
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

func bookDatapoint(payload string) *datura.Artifact {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("book")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))

	return artifact
}

const bookFramePayload = `{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a warmed book replay", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		frames := []string{
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":3.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":10.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
			`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":1.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
		}

		var (
			result         *datura.Artifact
			bestConfidence float64
		)

		for _, frame := range frames {
			datapoint := bookDatapoint(frame)
			result = signal.Measure(datapoint)
			confidence := datura.Peek[float64](result, "output", "confidence")

			if confidence > bestConfidence {
				bestConfidence = confidence
			}

			datapoint.Release()
		}

		Convey("It returns classifier output with non-uniform confidence", func() {
			So(result, ShouldNotBeNil)

			role, _ := result.Role()
			scope, _ := result.Scope()

			So(role, ShouldEqual, "book")
			So(scope, ShouldEqual, "update")
			So(datura.Peek[string](result, "channel"), ShouldEqual, "book")
			So(len(result.DecryptPayload()), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)
			So(bestConfidence, ShouldNotAlmostEqual, 1.0/3.0, 0.0001)

			result.Release()
		})
	})

	Convey("Given a single book frame without warmup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		datapoint := bookDatapoint(bookFramePayload)

		defer datapoint.Release()

		result := signal.Measure(datapoint)

		Convey("It returns a measurement artifact", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "channel"), ShouldEqual, "book")

			result.Release()
		})
	})
}

func TestMeasureBookFrames(testingTB *testing.T) {
	Convey("Given live-shaped kraken book fixtures", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		signal := NewSignal(ctx, newTestPool(testingTB), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		var result *datura.Artifact

		for tick := range 20 {
			datapoint := tests.NewFixture(tests.FixtureTypeBook).ToArtifact()
			datapoint.SetTimestamp(int64(tick + 1))

			result = signal.Measure(datapoint)
			datapoint.Release()
		}

		Convey("It should emit classifier output on a writable measurement artifact", func() {
			So(result, ShouldNotBeNil)
			So(len(result.DecryptPayload()), ShouldBeGreaterThan, 2)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0)

			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	frames := []string{
		bookFramePayload,
		`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100.0,"qty":3.0}],"asks":[{"price":101.0,"qty":10.0}]}]}`,
	}

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		var result *datura.Artifact

		for _, frame := range frames {
			datapoint := bookDatapoint(frame)
			result = signal.Measure(datapoint)
			datapoint.Release()
		}

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		result.Release()
		_ = signal.Close()
	}
}
