package depthflow

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

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func insertFeatures(signal *Signal, scope string, samples ...float64) {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	artifact := datura.Acquire("bookflow-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a loaded imbalance feature vector", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFeatures(signal, "BTC/EUR", loadedImbalancePayload()...)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify loaded imbalance and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "BTC/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a spoof trap feature vector", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFeatures(signal, "ETH/EUR", spoofTrapPayload()...)

		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should classify spoof trap and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "ETH/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given thinning touch depth", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFeatures(signal, "THIN/EUR", bookThinningPayload()...)

		result := signal.Measure(measurementQuery("THIN/EUR"))

		Convey("It should classify book thinning and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "THIN/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "THIN/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given balanced thick depth", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFeatures(signal, "NEUT/EUR", denseNeutralityPayload()...)

		result := signal.Measure(measurementQuery("NEUT/EUR"))

		Convey("It should classify dense neutrality and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "NEUT/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 4)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "NEUT/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given no feature artifacts in the tree", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery("SOL/EUR"))

		Convey("It should return nil without halting", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "SOL/EUR"), ShouldBeFalse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("BTC/EUR")
	payload := loadedImbalancePayload()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), NewTestTree())

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertFeatures(signal, "BTC/EUR", payload...)
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
