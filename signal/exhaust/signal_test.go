package exhaust

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

func decayPayload(
	lastPrice float64,
	bidDepths, askDepths, densities, spreads, pressures, imbalances []float64,
) []float64 {
	payload := []float64{lastPrice}

	series := [][]float64{
		bidDepths,
		askDepths,
		densities,
		spreads,
		pressures,
		imbalances,
	}

	for _, segment := range series {
		payload = append(payload, float64(len(segment)))
	}

	for _, segment := range series {
		payload = append(payload, segment...)
	}

	return payload
}

func insertDecayFeatures(signal *Signal, scope string, samples ...float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(samples...))

	signal.tree.Insert(artifact.Prefix(), artifact.Marshal())
	artifact.Release()
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given deteriorating long-side book history", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		bidDepths := []float64{20, 18, 16, 14, 12, 10, 8, 6}
		askDepths := []float64{10, 10, 10, 10, 10, 10, 10, 10}
		densities := []float64{8, 8, 8, 8, 8, 8, 8, 8}
		spreads := []float64{4, 4, 4, 4, 4, 4, 4, 4}
		pressures := []float64{0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2}
		imbalances := []float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}

		insertDecayFeatures(signal, "ETH/EUR", decayPayload(
			100, bidDepths, askDepths, densities, spreads, pressures, imbalances,
		)...)

		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should publish an exhaustion reading", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given pressure fade on the long side", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		pressures := []float64{0.9, 0.85, 0.8, 0.75, 0.7, 0.2, 0.1, -0.1}
		bidDepths := []float64{10, 10, 10, 10, 10, 10, 10, 10}
		askDepths := []float64{10, 10, 10, 10, 10, 10, 10, 10}
		densities := []float64{8, 8, 8, 8, 8, 8, 8, 8}
		spreads := []float64{4, 4, 4, 4, 4, 4, 4, 4}
		imbalances := []float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}

		insertDecayFeatures(signal, "BTC/EUR", decayPayload(
			100, bidDepths, askDepths, densities, spreads, pressures, imbalances,
		)...)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify thermal exhaustion from pressure fade", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given insufficient decay features", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		result := signal.Measure(measurementQuery("SOL/EUR"))

		Convey("It should return nil until history is populated", func() {
			So(result, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("ETH/EUR")
	payload := decayPayload(
		100,
		[]float64{20, 18, 16, 14, 12, 10, 8, 6},
		[]float64{10, 10, 10, 10, 10, 10, 10, 10},
		[]float64{8, 8, 8, 8, 8, 8, 8, 8},
		[]float64{4, 4, 4, 4, 4, 4, 4, 4},
		[]float64{0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2},
		[]float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1},
	)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertDecayFeatures(signal, "ETH/EUR", payload...)
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		_ = signal.Close()
	}
}
