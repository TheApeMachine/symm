package toxicity

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

func insertTreeFeatures(signal *Signal, role, scope string, samples ...float64) {
	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(encodeFloatPayload(samples...))

	if wire, err := artifact.Message().Marshal(); err == nil && len(wire) > 0 {
		signal.tree.Insert(artifact.Prefix(), wire)
	}

	artifact.Release()
}

func insertBookFeatures(signal *Signal, scope string, samples ...float64) {
	insertTreeFeatures(signal, "book", scope, samples...)
}

func insertOrderFeatures(signal *Signal, scope string, samples ...float64) {
	insertTreeFeatures(signal, "order", scope, samples...)
}

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func toxicBluffPayload() []float64 {
	return []float64{
		0, 0.1, 0, 0.1,
		80, 80,
		1, 4.5,
		0.15, 0.8, 0, 2,
		100,
	}
}

func liquidityVacuumPayload() []float64 {
	return []float64{
		0.3, 0.1, 0, 0,
		10, 10,
		0, 0,
		0.15, 0, 0, 2,
		50000,
	}
}

func hardSupportPayload() []float64 {
	return []float64{
		0, 0.1, 0, 0.1,
		80, 80,
		0, 0,
		0.15, 0, 0, 2,
		100,
	}
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given near-touch toxic churn above gate", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertBookFeatures(signal, "ETH/EUR", toxicBluffPayload()...)

		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should classify toxic bluff and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "ETH/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given cancel/fill asymmetry with fill flow", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertBookFeatures(signal, "BTC/EUR", liquidityVacuumPayload()...)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify liquidity vacuum and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "BTC/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given balanced depth with fills and no cancels", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertBookFeatures(signal, "SUPPORT/EUR", hardSupportPayload()...)

		result := signal.Measure(measurementQuery("SUPPORT/EUR"))

		Convey("It should classify hard support and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "SUPPORT/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "SUPPORT/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given order-role ingest at the prefix Measure seeks", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertOrderFeatures(signal, "ORD/EUR", liquidityVacuumPayload()...)

		result := signal.Measure(measurementQuery("ORD/EUR"))

		Convey("It should classify from order rows and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ORD/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "ORD/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a sparse tree at startup", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
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
	query := measurementQuery("BTC/EUR")
	payload := liquidityVacuumPayload()

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertBookFeatures(signal, "BTC/EUR", payload...)
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
