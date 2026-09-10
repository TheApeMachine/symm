package types

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"strconv"
	"testing"
	"time"
)

func TestEnvelopeMeasurements(t *testing.T) {
	Convey("Live grid input and persisted replay input include the same logic values", t, func() {
		at := time.Unix(100, 0)
		envelope := &Envelope{Key: "BTC/USD", CaptureID: CaptureIdentity{Run: "parity", Sequence: 1, Stream: "spot", StreamEpoch: 1},
			CVD:        data.NewMeasurement[float64]("flow", "BTC/USD", "cvd", at, at.Add(-time.Second)),
			Categories: []Category{{Symbol: "BTC/USD", At: at, Maturity: .75, Strength: 2, Confidence: .6}},
			Resonance:  &ResonanceArtifact{Symbol: "BTC/USD", At: at, ResolvedSteps: 4, Readout: []float64{1, -2}, ForwardCurve: []float64{.3, -.1}},
			Manifold:   &ManifoldState{At: at, Reading: sensorium.Reading{Divergence: 3, KuramotoR: .8}},
			Cognition:  &Cognition{Symbol: "BTC/USD", At: at, Sequence: "A B", Cohort: 4, Confidence: .8, Contrast: 2},
		}
		envelope.CVD.PutMetric(data.Metric[float64]{Label: "delta", Raw: 3})
		live := envelope.Measurements()
		captured, err := MeasurementsFromState(envelope.EncodeBytes())
		So(err, ShouldBeNil)
		So(len(live), ShouldEqual, 5)
		So(len(captured), ShouldEqual, len(live))
		for index, measurement := range live {
			measurement.Finalize()
			replay := captured[index]
			replay.Finalize()
			So(replay.Err, ShouldBeNil)
			So(replay.Source, ShouldEqual, measurement.Source)
			So(replay.Maturity, ShouldEqual, measurement.Maturity)
			So(replay.Metrics, ShouldResemble, measurement.Metrics)
		}
		So(live[1].Maturity, ShouldEqual, .75)
		So(live[2].Maturity, ShouldEqual, .75)
		envelope.Categories[0].Maturity = 0
		immature := envelope.LogicMeasurements()[0]
		immature.Finalize()
		So(immature.Maturity, ShouldEqual, 0)
	})
}

func TestEnvelopeEncodePrecursor(t *testing.T) {
	Convey("Every numerical input is durable without copying display or model state", t, func() {
		at := time.Unix(100, 0)
		envelope := &Envelope{Key: "BTC/USD", CaptureID: CaptureIdentity{Run: "precursor", Sequence: 1},
			CVD:      data.NewMeasurement[float64]("flow", "BTC/USD", "cvd", at, at),
			Manifold: &ManifoldState{At: at, Reading: sensorium.Reading{Divergence: 1}},
		}
		envelope.CVD.PutMetric(data.Metric[float64]{Label: "flow", Raw: 3})
		envelope.Manifold.MomRho = make([]float32, 4096)
		payload := envelope.EncodePrecursor()
		encoded := wire.GetRootAsEnvelopeState(payload, 0)
		So(encoded.Manifold(nil), ShouldBeNil)
		input, err := MeasurementsFromState(payload)
		So(err, ShouldBeNil)
		So(len(input), ShouldEqual, 2)
		So(input[0].Metrics["flow"].Raw, ShouldEqual, 3)
		So(input[1].Metrics["divergence"].Raw, ShouldEqual, 1)
	})
}

func BenchmarkEnvelopeEncodePrecursor(b *testing.B) {
	// The dashboard profile reported 412 quantities; retain that shape here.
	at := time.Unix(100, 0)
	envelope := &Envelope{Key: "BTC/USD", CaptureID: CaptureIdentity{Run: "bench", Sequence: 1},
		CVD: data.NewMeasurement[float64]("flow", "BTC/USD", "cvd", at, at)}
	for index := range 412 {
		envelope.CVD.PutMetric(data.Metric[float64]{Label: strconv.Itoa(index), Raw: float64(index)})
	}
	b.ReportAllocs()
	for b.Loop() {
		envelope.EncodePrecursor()
	}
}
