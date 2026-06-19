package sentiment

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

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
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

	InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a bullish feature vector", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFeatures(signal, "A/EUR", 0.7, 0.02, 0.55, 1, 0.02)

		result := signal.Measure(measurementQuery("A/EUR"))

		Convey("It should classify a risk-on surge and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "A/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "A/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a weak breadth vector with a local leader", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFeatures(signal, "LEAD/EUR", 0.1, 0.6, 0.55, 1, 0.6)

		result := signal.Measure(measurementQuery("LEAD/EUR"))

		Convey("It should classify a divergent move and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "LEAD/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "LEAD/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given weak breadth without leadership", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFeatures(signal, "SLUMP/EUR", 0.2, -0.05, 0.5, 0, -0.05)

		result := signal.Measure(measurementQuery("SLUMP/EUR"))

		Convey("It should classify a systemic slump and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "SLUMP/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "SLUMP/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery("NEW/EUR"))

		Convey("It should return nil without error", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "NEW/EUR"), ShouldBeFalse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("A/EUR")

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), NewTestTree())

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertFeatures(signal, "A/EUR", 0.7, 0.02, 0.55, 1, 0.02)
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, "A/EUR") {
			b.Fatal("InsertMeasurement did not index measurement/A/EUR")
		}

		result.Release()
		_ = signal.Close()
	}
}
