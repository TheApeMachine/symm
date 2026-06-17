package sentiment

import (
	"context"
	"encoding/binary"
	"fmt"
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

func insertFeatures(signal *Signal, scope string, breadth, change, surgeThreshold, leader, move float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(
		breadth,
		change,
		surgeThreshold,
		leader,
		move,
	))

	signal.tree.Insert(artifact.Prefix(), artifact.Marshal())
	artifact.Release()
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a bullish feature vector", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertFeatures(signal, "A/EUR", 0.7, 0.02, 0.55, 1, 0.02)

		result := signal.Measure(measurementQuery("A/EUR"))

		Convey("It should classify a risk-on surge", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "A/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a weak breadth vector with a local leader", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertFeatures(signal, "LEAD/EUR", 0.1, 0.6, 0.55, 1, 0.6)

		result := signal.Measure(measurementQuery("LEAD/EUR"))

		Convey("It should classify a divergent move", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
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

	Convey("Given a non-finite breadth input", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertFeatures(signal, "A/EUR", math.NaN(), 0.02, 0.55, 1, 0.02)

		result := signal.Measure(measurementQuery("A/EUR"))

		Convey("It should not panic on invalid breadth", func() {
			So(result, ShouldNotBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(context.Background(), newTestPool(nil))

	if signal == nil {
		b.Fatal("NewSignal returned nil")
	}

	for index := range 16 {
		scope := fmt.Sprintf("SYM%d/EUR", index)
		insertFeatures(signal, scope, 0.6, 0.02, 0.55, 1, float64(index)*0.01)
	}

	query := measurementQuery("SYM0/EUR")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}
	}
}
