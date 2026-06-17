package logic

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestMeasurementFromArtifact(testingTB *testing.T) {
	Convey("Given a JSON measurement payload", testingTB, func() {
		expected := Measurement{
			Source:     SourceFluid,
			Symbol:     "ETH/USD",
			Price:      100,
			Strength:   0.5,
			Confidence: 0.8,
		}
		payload, marshalErr := json.Marshal(expected)

		So(marshalErr, ShouldBeNil)

		artifact := datura.Acquire("fluid", datura.Artifact_Type_json)
		artifact.WithRole("measurement")
		artifact.WithScope("ETH/USD")
		artifact.WithPayload(payload)

		measurement, ok := MeasurementFromArtifact("fluid", artifact)

		Convey("It should decode the payload", func() {
			So(ok, ShouldBeTrue)
			So(measurement.Source, ShouldEqual, SourceFluid)
			So(measurement.Symbol, ShouldEqual, "ETH/USD")
			So(measurement.Confidence, ShouldEqual, 0.8)
		})

		artifact.Release()
	})

	Convey("Given classifier metadata on a signal artifact", testingTB, func() {
		artifact := datura.Acquire("hawkes", datura.Artifact_Type_json)
		artifact.WithRole("measurement")
		artifact.WithScope("BTC/EUR")
		artifact.WithAttribute("classifier.category", 2)
		artifact.WithAttribute("classifier.confidence", 0.71)
		artifact.WithAttribute("classifier.strength", 1.25)

		measurement, ok := MeasurementFromArtifact("hawkes", artifact)

		Convey("It should derive the measurement without JSON payload", func() {
			So(ok, ShouldBeTrue)
			So(measurement.Source, ShouldEqual, SourceHawkes)
			So(measurement.Symbol, ShouldEqual, "BTC/EUR")
			So(measurement.Category, ShouldEqual, CategorySaturation)
			So(measurement.Confidence, ShouldEqual, 0.71)
			So(measurement.Strength, ShouldEqual, 1.25)
		})

		artifact.Release()
	})

	Convey("Given a binary classifier payload", testingTB, func() {
		payload := make([]byte, 8)
		binary.BigEndian.PutUint64(payload, math.Float64bits(2))

		artifact := datura.Acquire("hawkes", datura.Artifact_Type_json)
		artifact.WithScope("BTC/EUR")
		artifact.WithPayload(payload)

		_, ok := MeasurementFromArtifact("hawkes", artifact)

		Convey("It should not treat the payload as JSON", func() {
			So(ok, ShouldBeFalse)
		})

		artifact.Release()
	})
}

func BenchmarkMeasurementFromArtifact(benchmark *testing.B) {
	artifact := datura.Acquire("hawkes", datura.Artifact_Type_json)
	artifact.WithScope("BTC/EUR")
	artifact.WithAttribute("classifier.category", 2)
	artifact.WithAttribute("classifier.confidence", 0.71)

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for benchmark.Loop() {
		_, _ = MeasurementFromArtifact("hawkes", artifact)
	}

	artifact.Release()
}
