package pumpdump

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

func insertVerticalityReplay(signal *Signal, query *datura.Artifact, samples ...float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	scope, _ := query.Scope()
	artifact.WithRole("measurement")
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(samples...))

	signal.tree.Insert(query.Prefix(), artifact.Marshal())
	artifact.Release()
}

func treeHasMeasurement(signal *Signal, prefix []byte) bool {
	if signal == nil || len(prefix) == 0 {
		return false
	}

	for range signal.tree.Seek(prefix) {
		return true
	}

	return false
}

func verticalIgnitionPayload() []float64 {
	return []float64{1.1, 3.1, 3.1, 0.0}
}

func coiledCompressionPayload() []float64 {
	return []float64{1.5, 0.05, 2.0, 0.5}
}

func organicTrendPayload() []float64 {
	return []float64{1.1, 3.1, 0.1, 0.0}
}

func fadedExhaustionPayload() []float64 {
	return []float64{0.5, 0.1, 0.5, 0.0}
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a vertical ignition feature vector", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		query := measurementQuery("ETH/EUR")

		defer query.Release()

		insertVerticalityReplay(signal, query, verticalIgnitionPayload()...)

		replayPrefix := append([]byte(nil), query.Prefix()...)

		result := signal.Measure(query)

		Convey("It should classify vertical ignition from the replay prefix", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, replayPrefix), ShouldBeTrue)
		})
	})

	Convey("Given spread compression with low precursor", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		query := measurementQuery("BTC/EUR")

		defer query.Release()

		insertVerticalityReplay(signal, query, coiledCompressionPayload()...)

		replayPrefix := append([]byte(nil), query.Prefix()...)

		result := signal.Measure(query)

		Convey("It should classify coiled compression from the replay prefix", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, replayPrefix), ShouldBeTrue)
		})
	})

	Convey("Given steady momentum without vertical lift", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		query := measurementQuery("TREND/EUR")

		defer query.Release()

		insertVerticalityReplay(signal, query, organicTrendPayload()...)

		replayPrefix := append([]byte(nil), query.Prefix()...)

		result := signal.Measure(query)

		Convey("It should classify organic trend from the replay prefix", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, replayPrefix), ShouldBeTrue)
		})
	})

	Convey("Given fading volume lift with flat precursor", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		query := measurementQuery("FADE/EUR")

		defer query.Release()

		insertVerticalityReplay(signal, query, fadedExhaustionPayload()...)

		replayPrefix := append([]byte(nil), query.Prefix()...)

		result := signal.Measure(query)

		Convey("It should classify faded exhaustion from the replay prefix", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, replayPrefix), ShouldBeTrue)
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		query := measurementQuery("NEW/EUR")

		defer query.Release()

		replayPrefix := append([]byte(nil), query.Prefix()...)

		result := signal.Measure(query)

		Convey("It should leave the query unclassified without replay rows", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldEqual, 0)
			So(treeHasMeasurement(signal, replayPrefix), ShouldBeFalse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("BTC/EUR")
	payload := coiledCompressionPayload()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), NewTestTree())

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertVerticalityReplay(signal, query, payload...)

		replayPrefix := append([]byte(nil), query.Prefix()...)

		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if datura.Peek[int](result, "classifier.category") <= 0 {
			b.Fatal("Measure did not classify coiled compression")
		}

		if !treeHasMeasurement(signal, replayPrefix) {
			b.Fatal("tree replay did not index the measurement query prefix")
		}

		_ = signal.Close()
	}

	query.Release()
}
