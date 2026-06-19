package cvd

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

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

func measurementQuery(scope string) *datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return acquired
}

func encodeFloatPayload(samples ...float64) []byte {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload
}

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func insertTradeFlow(signal *Signal, scope string, samples ...float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole("trade")
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(samples...))

	if wire, err := artifact.Message().Marshal(); err == nil && len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func absorptionFlowPayload() []float64 {
	return []float64{
		200, 0, 4, 0, 50,
		50, 50.001, 50, 50.001,
	}
}

func driveFlowPayload() []float64 {
	return []float64{
		500, 0, 5, 0, 100,
		100, 100.01, 100.02, 100.03, 100.04,
	}
}

func balanceFlowPayload() []float64 {
	return []float64{
		100, 100, 4, 0, 50,
		100, 100.01, 99.99, 100.02, 99.98,
	}
}

func starvationFlowPayload() []float64 {
	return []float64{
		5, 0, 2, 50, 5,
		100, 100.01,
	}
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given aggressive buy flow with flat price", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertTradeFlow(signal, "ETH/EUR", absorptionFlowPayload()...)

		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should classify hidden absorption and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "ETH/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given aggressive buy flow with rising price", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertTradeFlow(signal, "BTC/EUR", driveFlowPayload()...)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify aggressive drive and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "BTC/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given balanced two-sided flow with mixed drift", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertTradeFlow(signal, "BAL/EUR", balanceFlowPayload()...)

		result := signal.Measure(measurementQuery("BAL/EUR"))

		Convey("It should classify stochastic balance and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BAL/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "BAL/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given thin flow below gross floor", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertTradeFlow(signal, "STAR/EUR", starvationFlowPayload()...)

		result := signal.Measure(measurementQuery("STAR/EUR"))

		Convey("It should classify volume starvation and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "STAR/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "STAR/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery("XRP/EUR"))

		Convey("It should return nil without error", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "XRP/EUR"), ShouldBeFalse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("BTC/EUR")
	payload := driveFlowPayload()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertTradeFlow(signal, "BTC/EUR", payload...)
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, "BTC/EUR") {
			b.Fatal("InsertMeasurement did not index measurement/BTC/EUR")
		}

		result.Release()
		_ = signal.Close()
	}
}
