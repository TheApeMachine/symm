package depthflow

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	feed "github.com/theapemachine/symm/signal"
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

func insertFeatures(signal *Signal, scope string, samples ...float64) {
	payload, err := json.Marshal(samples)

	if err != nil {
		panic(err)
	}

	artifact := datura.Acquire("bookflow-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	feed.InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func loadedImbalancePayload() []float64 {
	weightedHistory := []float64{0.8, 0.82, 0.84, 0.86, 0.88, 0.9, 0.92, 0.94}
	level1History := []float64{0.7, 0.72, 0.74, 0.76, 0.78, 0.8, 0.82, 0.84}
	flatHistory := []float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}

	samples := []float64{
		0.9, 0.85, 0.5, 1, 100, 2, 50, 0.8,
		float64(len(weightedHistory)),
		float64(len(level1History)),
		float64(len(flatHistory)),
	}
	samples = append(samples, weightedHistory...)
	samples = append(samples, level1History...)
	samples = append(samples, flatHistory...)

	return samples
}

func spoofTrapPayload() []float64 {
	weightedHistory := []float64{0.85, 0.86, 0.87, 0.88, 0.89, 0.9, 0.91, 0.92}
	level1History := []float64{0.2, 0.22, 0.24, 0.26, 0.28, 0.3, 0.32, 0.34}
	flatHistory := []float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}

	samples := []float64{
		0.9, 0.2, 0.5, 1, 50, 2, 40, -0.2,
		float64(len(weightedHistory)),
		float64(len(level1History)),
		float64(len(flatHistory)),
	}
	samples = append(samples, weightedHistory...)
	samples = append(samples, level1History...)
	samples = append(samples, flatHistory...)

	return samples
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a loaded imbalance feature vector", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertFeatures(signal, "BTC/EUR", loadedImbalancePayload()...)

		result := signal.Measure(measurementQuery("BTC/EUR"))

		Convey("It should classify loaded imbalance", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "BTC/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a spoof trap feature vector", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		insertFeatures(signal, "ETH/EUR", spoofTrapPayload()...)

		result := signal.Measure(measurementQuery("ETH/EUR"))

		Convey("It should classify spoof trap", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given no feature artifacts in the tree", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		result := signal.Measure(measurementQuery("SOL/EUR"))

		Convey("It should return nil without halting", func() {
			So(result, ShouldBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
	query := measurementQuery("BTC/EUR")

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), pool)

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertFeatures(signal, "BTC/EUR", loadedImbalancePayload()...)

		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}
	}
}
