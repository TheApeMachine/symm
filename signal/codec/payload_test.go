package codec

import (
	"encoding/binary"
	"io"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestEncodePayload(testingTB *testing.T) {
	Convey("Given float64 samples", testingTB, func() {
		payload := EncodePayload(1.5, 2.25)

		Convey("When the payload is decoded as big-endian float64", func() {
			first := math.Float64frombits(binary.BigEndian.Uint64(payload[:8]))
			second := math.Float64frombits(binary.BigEndian.Uint64(payload[8:]))

			Convey("It should preserve sample order", func() {
				So(first, ShouldEqual, 1.5)
				So(second, ShouldEqual, 2.25)
			})
		})
	})
}

func TestReadFeatureArtifact(testingTB *testing.T) {
	Convey("Given a feature artifact", testingTB, func() {
		payload := EncodePayload(3, 4)
		artifact := datura.Acquire("features", datura.Artifact_Type_json).
			WithPayload(payload)

		buffer := make([]byte, len(payload))

		Convey("When ReadFeatureArtifact copies the payload", func() {
			readCount, readErr := ReadFeatureArtifact(buffer, artifact)

			Convey("It should fill the buffer and return EOF", func() {
				So(readErr, ShouldEqual, io.EOF)
				So(readCount, ShouldEqual, len(payload))
				So(buffer, ShouldResemble, payload)
			})
		})
	})
}

func TestMaxFeatureFloats(testingTB *testing.T) {
	Convey("Given a header float count below the ceiling", testingTB, func() {
		maxFloats := MaxFeatureFloats("depth-features", "features", "BTC/USD", 4)

		Convey("It should return the feature ceiling", func() {
			So(maxFloats, ShouldEqual, maxFeatureFloatCount)
		})
	})
}

func TestTrimLargestFloats(testingTB *testing.T) {
	Convey("Given more peers than the budget allows", testingTB, func() {
		trimmed := TrimLargestFloats([]float64{10, 1, 5, 2}, 2)

		Convey("It should keep the smallest values", func() {
			So(trimmed, ShouldResemble, []float64{1, 2})
		})
	})
}

func BenchmarkEncodePayload(benchmark *testing.B) {
	for benchmark.Loop() {
		_ = EncodePayload(1, 2, 3, 4, 5)
	}
}
