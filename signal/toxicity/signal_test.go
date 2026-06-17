package toxicity

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
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

func measurementQuery(scope string) datura.Artifact {
	acquired := datura.Acquire("trader", datura.Artifact_Type_json)
	acquired.WithRole("measurement")
	acquired.WithScope(scope)

	return *acquired
}

func encodeFloatPayload(samples ...float64) []byte {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload
}

func insertBookFeatures(signal *Signal, scope string, samples ...float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole("book")
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(samples...))

	signal.tree.Insert(artifact.Prefix(), artifact.Marshal())
	artifact.Release()
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given near-touch toxic churn above gate", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertBookFeatures(signal, "ETH/EUR",
			0, 0.1, 0, 0.1,
			80, 80,
			1, 4.5,
			0.15, 0.8, 0, 2,
			100,
		)

		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should classify toxic bluff", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given cancel/fill asymmetry with fill flow", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertBookFeatures(signal, "BTC/EUR",
			0.3, 0.1, 0, 0,
			10, 10,
			0, 0,
			0.15, 0, 0, 2,
			50000,
		)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify liquidity vacuum", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given balanced depth with fills and no cancels", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertBookFeatures(signal, "SUPPORT/EUR",
			0, 0.1, 0, 0.1,
			80, 80,
			0, 0,
			0.15, 0, 0, 2,
			100,
		)

		result := signal.Measure(measurementQuery("SUPPORT/EUR"))

		Convey("It should classify hard support", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		result := signal.Measure(measurementQuery("NEW/EUR"))

		Convey("It should return nil without error", func() {
			So(result, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("BTC/EUR")
	payload := []float64{
		0.3, 0.1, 0, 0,
		10, 10,
		0, 0,
		0.15, 0, 0, 2,
		50000,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertBookFeatures(signal, "BTC/EUR", payload...)
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		_ = signal.Close()
	}
}
