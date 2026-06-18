package exhaust

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	. "github.com/theapemachine/symm/signal"
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

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
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

	InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func mechanicalCollapsePayload() []float64 {
	return decayPayload(
		100,
		[]float64{20, 18, 16, 14, 12, 10, 8, 6},
		[]float64{10, 10, 10, 10, 10, 10, 10, 10},
		[]float64{8, 8, 8, 8, 8, 8, 8, 8},
		[]float64{4, 4, 4, 4, 4, 4, 4, 4},
		[]float64{0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2},
		[]float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1},
	)
}

func fragileExpansionPayload() []float64 {
	return decayPayload(
		100,
		[]float64{10, 10, 10, 10, 10, 10, 10, 10},
		[]float64{10, 10, 10, 10, 10, 10, 10, 10},
		[]float64{8, 8, 8, 8, 8, 8, 8, 8},
		[]float64{4, 4, 4, 4, 4, 4, 4, 12},
		[]float64{0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2},
		[]float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1},
	)
}

func thermalExhaustionPayload() []float64 {
	return decayPayload(
		100,
		[]float64{10, 10, 10, 10, 10, 10, 10, 10},
		[]float64{10, 10, 10, 10, 10, 10, 10, 10},
		[]float64{8, 8, 8, 8, 8, 8, 8, 8},
		[]float64{4, 4, 4, 4, 4, 4, 4, 4},
		[]float64{0.9, 0.85, 0.8, 0.75, 0.7, 0.2, 0.1, -0.1},
		[]float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1},
	)
}

func activeReversalPayload() []float64 {
	return decayPayload(
		100,
		[]float64{10, 10, 10, 10, 10, 10, 10, 10},
		[]float64{10, 10, 10, 10, 10, 10, 10, 10},
		[]float64{8, 8, 8, 8, 8, 8, 8, 8},
		[]float64{2, 2, 2, 2, 2, 2, 2, 2},
		[]float64{0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2},
		[]float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, -0.8},
	)
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given deteriorating long-side book history", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertDecayFeatures(signal, "ETH/EUR", mechanicalCollapsePayload()...)

		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should classify mechanical collapse and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "ETH/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given widening spreads against a stable book", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertDecayFeatures(signal, "FRAG/EUR", fragileExpansionPayload()...)

		result := signal.Measure(measurementQuery("FRAG/EUR"))

		Convey("It should classify fragile expansion and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "FRAG/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "FRAG/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given pressure fade on the long side", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertDecayFeatures(signal, "BTC/EUR", thermalExhaustionPayload()...)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify thermal exhaustion and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "BTC/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a long-side imbalance flip", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertDecayFeatures(signal, "REV/EUR", activeReversalPayload()...)

		result := signal.Measure(measurementQuery("REV/EUR"))

		Convey("It should classify active reversal and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "REV/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "REV/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given insufficient decay features", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery("SOL/EUR"))

		Convey("It should return nil until history is populated", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "SOL/EUR"), ShouldBeFalse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("ETH/EUR")
	payload := mechanicalCollapsePayload()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), NewTestTree())

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertDecayFeatures(signal, "ETH/EUR", payload...)
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, "ETH/EUR") {
			b.Fatal("InsertMeasurement did not index measurement/ETH/EUR")
		}

		result.Release()
		_ = signal.Close()
	}
}
