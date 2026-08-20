package telemetry

import (
	"testing"

	flatbuffers "github.com/google/flatbuffers/go"
	. "github.com/smartystreets/goconvey/convey"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func TestEncodeBatch(t *testing.T) {
	Convey("Given ordered schema-tagged frames", t, func() {
		frames := []*wire.FrameT{
			{Type: wire.FrameTickFrame, Value: &wire.TickFrameT{Count: 1}},
			{Type: wire.FrameTickFrame, Value: &wire.TickFrameT{Count: 2}},
		}

		Convey("It should produce one canonical FlatBuffers batch without an outer copy", func() {
			encoded := EncodeBatch(frames)
			defer encoded.Release()
			So(flatbuffers.BufferHasIdentifier(encoded.Bytes, BatchIdentifier), ShouldBeTrue)
			batch := wire.GetRootAsBatch(encoded.Bytes, 0).UnPack()
			So(batch.Sequence, ShouldBeGreaterThan, uint64(0))
			So(batch.Frames, ShouldHaveLength, 2)
			So(batch.Frames[0].Frame.Type, ShouldEqual, wire.FrameTickFrame)
			So(batch.Frames[0].Frame.Value.(*wire.TickFrameT).Count, ShouldEqual, int64(1))
			So(batch.Frames[1].Frame.Value.(*wire.TickFrameT).Count, ShouldEqual, int64(2))
		})
	})
}

func TestDecode(t *testing.T) {
	Convey("Given one standalone semantic envelope", t, func() {
		encoded := Encode(&wire.FrameT{
			Type:  wire.FrameTickFrame,
			Value: &wire.TickFrameT{Count: 9},
		})

		Convey("It should validate and decode the schema identifier", func() {
			envelope, err := Decode(encoded)
			So(err, ShouldBeNil)
			So(envelope.Frame.Type, ShouldEqual, wire.FrameTickFrame)
			So(envelope.Frame.Value.(*wire.TickFrameT).Count, ShouldEqual, int64(9))
		})
	})
}

func BenchmarkEncodeBatch(b *testing.B) {
	rows := make([]*wire.MeasurementT, 256)

	for index := range rows {
		rows[index] = &wire.MeasurementT{
			Id:     "hawkes-intensity",
			Source: "hawkes",
			Symbol: "BTC/USD",
			Metrics: []*wire.MetricT{{
				Name: "intensity",
				Raw:  float64(index),
			}},
		}
	}

	frames := []*wire.FrameT{{
		Type:  wire.FrameMeasurementsFrame,
		Value: &wire.MeasurementsFrameT{Rows: rows},
	}}
	b.ReportAllocs()

	for b.Loop() {
		encoded := EncodeBatch(frames)
		encoded.Release()
	}
}
