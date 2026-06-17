package signal

import (
	"encoding/binary"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
)

func init() {
	viper.Set("signals.feed_ring_capacity", 64)
}

func TestEncodePayload(testingTB *testing.T) {
	Convey("Given float samples", testingTB, func() {
		payload := EncodePayload(1.5, 2.5)

		Convey("It should encode eight bytes per sample", func() {
			So(len(payload), ShouldEqual, 16)
			So(math.Float64frombits(binary.BigEndian.Uint64(payload[0:8])), ShouldEqual, 1.5)
			So(math.Float64frombits(binary.BigEndian.Uint64(payload[8:16])), ShouldEqual, 2.5)
		})
	})
}

func TestReadFeatureArtifact(testingTB *testing.T) {
	Convey("Given a feature artifact", testingTB, func() {
		artifact := datura.Acquire("frame-test", datura.Artifact_Type_json)
		artifact.WithRole("features")
		artifact.WithScope("ETH/USD")
		artifact.WithPayload(EncodePayload(1, 2, 3))

		buffer := make([]byte, FeatureFrameSize)

		Convey("It should marshal into the frame buffer", func() {
			readCount, readErr := ReadFeatureArtifact(buffer, artifact)

			So(readErr, ShouldBeNil)
			So(readCount, ShouldBeGreaterThan, 0)
			So(readCount, ShouldBeLessThanOrEqualTo, len(buffer))
		})
	})
}

func TestMaxFeatureFloats(testingTB *testing.T) {
	Convey("Given a feature identity", testingTB, func() {
		maxFloats := MaxFeatureFloats("cohort-features", "features", "ETH/USD", 5)

		Convey("It should reserve at least the header floats", func() {
			So(maxFloats, ShouldBeGreaterThanOrEqualTo, 5)
		})
	})
}

func TestTrimHistoryTails(testingTB *testing.T) {
	Convey("Given parallel histories over budget", testingTB, func() {
		trimmed := TrimHistoryTails(
			[][]float64{{1, 2, 3}, {4, 5}, {6, 7, 8, 9}},
			5,
		)

		total := 0

		for _, history := range trimmed {
			total += len(history)
		}

		Convey("It should fit the float budget", func() {
			So(total, ShouldBeLessThanOrEqualTo, 5)
		})
	})
}

func TestTouchSpread(testingTB *testing.T) {
	Convey("Given a two-price trade window", testingTB, func() {
		spread, spreadOK := TouchSpread([]float64{100, 101})

		Convey("It should return spread in basis points", func() {
			So(spreadOK, ShouldBeTrue)
			So(spread, ShouldAlmostEqual, 99.50248756218905, 0.0001)
		})
	})
}

func BenchmarkEncodePayload(b *testing.B) {
	samples := make([]float64, 128)

	for index := range samples {
		samples[index] = float64(index)
	}

	b.ResetTimer()

	for b.Loop() {
		_ = EncodePayload(samples...)
	}
}
