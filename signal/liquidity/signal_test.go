package liquidity

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/equation"
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

func treeHasMeasurement(signal *Signal, scope string) bool {
	prefix := "measurement/" + scope

	for range signal.tree.Seek([]byte(prefix)) {
		return true
	}

	return false
}

func depthFeaturesPayload(
	scaledQuoteVol float64,
	peers []float64,
	relativeVolume float64,
	baselineReady bool,
) []float64 {
	samples := []float64{scaledQuoteVol, float64(len(peers))}
	samples = append(samples, peers...)
	samples = append(samples, relativeVolume)

	baselineFlag := 0.0

	if baselineReady {
		baselineFlag = 1
	}

	samples = append(samples, baselineFlag)

	return samples
}

func encodeFeaturePayload(samples ...float64) []byte {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		offset := index * 8
		binary.BigEndian.PutUint64(payload[offset:offset+8], math.Float64bits(sample))
	}

	return payload
}

func insertFeatures(signal *Signal, scope string, samples ...float64) {
	payload := encodeFeaturePayload(samples...)

	artifact := datura.Acquire("depth-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a peak-scarcity symbol", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFeatures(signal, "THIN/EUR",
			depthFeaturesPayload(50, []float64{1100, 950, 50}, 1, false)...,
		)

		result := signal.Measure(measurementQuery("THIN/EUR"))

		Convey("It should classify extreme scarcity and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "THIN/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 1)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "THIN/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a cross-section near the median", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFeatures(signal, "MID/EUR",
			depthFeaturesPayload(900, []float64{800, 900, 1200}, 1, false)...,
		)

		result := signal.Measure(measurementQuery("MID/EUR"))

		Convey("It should classify median depth and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "MID/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 2)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "MID/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given a cross-section with deep and thin peers", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		insertFeatures(signal, "ALT/EUR",
			depthFeaturesPayload(1200, []float64{800, 900, 1200}, 1, false)...,
		)

		result := signal.Measure(measurementQuery("ALT/EUR"))

		Convey("It should classify robust liquidity and publish to the tree", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ALT/EUR")
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(datura.Peek[float64](result, "classifier.confidence"), ShouldBeGreaterThan, 0)
			So(treeHasMeasurement(signal, "ALT/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given market-wide high absolute volume", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scaledQuoteVol, scaledPeers := equation.AbsoluteScaledVolumes(
			300,
			[]float64{280, 290, 300},
			3,
			true,
		)

		insertFeatures(signal, "BASE/EUR",
			depthFeaturesPayload(scaledQuoteVol, scaledPeers, 3, true)...,
		)

		result := signal.Measure(measurementQuery("BASE/EUR"))

		Convey("It should suppress relative scarcity above baseline", func() {
			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier.category"), ShouldEqual, 3)
			So(treeHasMeasurement(signal, "BASE/EUR"), ShouldBeTrue)
			result.Release()
		})
	})

	Convey("Given fewer than two universe symbols", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery("SOLO/EUR"))

		Convey("It should withhold until a peer universe exists", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "SOLO/EUR"), ShouldBeFalse)
		})
	})

	Convey("Given no feature artifacts in the tree", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), NewTestTree())
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		result := signal.Measure(measurementQuery("NOFEAT/EUR"))

		Convey("It should return nil without halting", func() {
			So(result, ShouldBeNil)
			So(treeHasMeasurement(signal, "NOFEAT/EUR"), ShouldBeFalse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	query := measurementQuery("SYM0/EUR")
	payload := depthFeaturesPayload(
		1200,
		[]float64{500, 550, 600, 650, 700, 750, 800, 850, 900, 950, 1000, 1050, 1100, 1150, 1200, 1250},
		1,
		false,
	)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), NewTestTree())

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		insertFeatures(signal, "SYM0/EUR", payload...)
		result := signal.Measure(query)

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if !treeHasMeasurement(signal, "SYM0/EUR") {
			b.Fatal("InsertMeasurement did not index measurement/SYM0/EUR")
		}

		result.Release()
		_ = signal.Close()
	}
}
