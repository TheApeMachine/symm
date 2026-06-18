package pumpdump

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

func insertObservation(signal *Signal, role, scope string, payload []float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(payload...))

	signal.tree.Insert(artifact.Prefix(), artifact.Marshal())
	artifact.Release()
}

func insertFeatureObservation(signal *Signal, scope string, payload []float64) {
	insertObservation(signal, "measurement", scope, payload)
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a verticality feature vector with volume lift", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertFeatureObservation(signal, "ETH/EUR", []float64{4.0, 0.2, 0.5, 6.0})

		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should classify without error", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given spread compression context", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertFeatureObservation(signal, "BTC/EUR", []float64{1.5, 0.05, 2.0, 0.5})

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should measure without error", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a second scope with compressed spread replay", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertFeatureObservation(signal, "ETH/EUR", []float64{1.5, 0.05, 2.0, 0.5})

		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should preserve scope on replay", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/EUR")
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(context.Background(), newTestPool(nil))

	if signal == nil {
		b.Fatal("NewSignal returned nil")
	}

	for index := range 32 {
		insertFeatureObservation(signal, "ETH/EUR", []float64{
			float64(index%5) + 1,
			0.1,
			0.2,
			float64(index),
		})
	}

	query := measurementQuery("ETH/EUR")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}
	}
}
