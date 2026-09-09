package hindsight_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestArtifactWitnessRehearsalInput(t *testing.T) {
	Convey("Rehearsal reuses the captured producer inputs without action or outcome labels", t, func() {
		capture := hindsight.CaptureIdentity{Run: "test", Sequence: 7, Stream: "spot", StreamEpoch: 1}
		at := time.Unix(1700000000, 0)
		measurement := data.NewMeasurement[float64]("precursor", "BTC/USD", "depthflow", at, at.Add(-time.Second))
		measurement.Metadata = map[string]float64{
			data.MetadataSupport: 4, data.MetadataDivergence: 2, data.MetadataNoiseVariance: 2,
		}
		measurement.Provenance = map[string]string{"venue": "captured"}
		measurement.Finalize()
		measurement.PutMetric(data.Metric[float64]{Label: "flow", Raw: 12})
		envelope := &types.Envelope{Key: "BTC/USD", CaptureID: capture, DepthFlow: measurement}
		witness := hindsight.ArtifactWitness{Envelope: hindsight.EnvelopeRef{Origin: capture}, Payload: envelope.EncodePrecursor()}
		input, err := witness.RehearsalInput()
		So(err, ShouldBeNil)
		So(input.Symbol, ShouldEqual, "BTC/USD")
		So(len(input.Measurements), ShouldEqual, 1)
		So(input.Measurements[0].Source, ShouldEqual, "depthflow")
		So(input.Measurements[0].Metrics["flow"].Raw, ShouldEqual, 12)
		So(input.Measurements[0].Maturity, ShouldEqual, .75)
		input.Measurements[0].Finalize()
		So(input.Measurements[0].Maturity, ShouldEqual, .75)
		So(input.Measurements[0].SNR, ShouldEqual, 2)
		So(input.Measurements[0].Provenance, ShouldResemble, measurement.Provenance)
		So(input.Measurements[0].From.Equal(measurement.From), ShouldBeTrue)

		Convey("An identity mismatch or malformed payload is an explicit failure", func() {
			witness.Envelope.Origin.Sequence++
			_, err := witness.RehearsalInput()
			So(err, ShouldNotBeNil)
			witness.Payload = []byte{1}
			_, err = witness.RehearsalInput()
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkArtifactWitnessRehearsalInput(b *testing.B) {
	capture := hindsight.CaptureIdentity{Run: "bench", Sequence: 1, Stream: "spot", StreamEpoch: 1}
	measurement := data.NewMeasurement[float64]("precursor", "BTC/USD", "depthflow", time.Now(), time.Now())
	measurement.PutMetric(data.Metric[float64]{Label: "flow", Raw: 12})
	envelope := &types.Envelope{Key: "BTC/USD", CaptureID: capture, DepthFlow: measurement}
	witness := hindsight.ArtifactWitness{Envelope: hindsight.EnvelopeRef{Origin: capture}, Payload: envelope.EncodePrecursor()}
	b.ReportAllocs()

	for b.Loop() {
		if _, err := witness.RehearsalInput(); err != nil {
			b.Fatal(err)
		}
	}
}
