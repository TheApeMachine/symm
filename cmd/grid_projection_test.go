package cmd

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/types"
)

func TestGridNodeProject(t *testing.T) {
	Convey("Given scalar, vector and resident solver readouts", t, func() {
		node := &gridNode{Grid: learning.NewGrid()}
		envelope := types.NewEnvelope(types.EnvelopeTicker)
		envelope.TickerData.Symbol = "TEST/USD"
		entropy := 2.0
		envelope.Cognition = &types.Cognition{At: time.Unix(1, 0), Confidence: 0.75, EntropyBits: &entropy,
			Classes: []types.CognitionClass{{Name: "class", Probability: 0.8}}, Predictions: map[string]float64{"next": 0.6}}
		envelope.Resonance = &types.ResonanceArtifact{At: time.Unix(1, 0), Readout: []float64{2, -3}, ForwardCurve: []float64{1, 2}, Confidence: 0.5}
		envelope.Manifold = &types.ManifoldState{At: time.Unix(1, 0), Version: 1, Reading: sensorium.Reading{Divergence: -4, KuramotoR: 0.4}}
		output := [3]*data.Measurement[float64]{}
		So(node.project(envelope, output[:]), ShouldBeNil)
		So(output[0].Metrics["Confidence"].Raw, ShouldEqual, 0.75)
		So(output[0].Metrics["EntropyBits"].Raw, ShouldEqual, 2)
		So(output[0].Metrics["class.class"].Raw, ShouldEqual, 0.8)
		So(output[0].Metrics["prediction.next"].Raw, ShouldEqual, 0.6)
		So(output[1].Metrics["readout.1"].Raw, ShouldEqual, -3)
		So(output[2].Metrics["Divergence"].Raw, ShouldEqual, -4)
		So(output[2].Metrics, ShouldHaveLength, 6)
		So(node.Grid.Step(output[:]), ShouldBeNil)
		coordinates := output[0].Metrics["Confidence"].Coordinates
		prior := output[0]

		Convey("Later projections reuse storage, retain coordinates and omit absent optionals", func() {
			envelope.Cognition.EntropyBits = nil
			envelope.Cognition.Confidence = 0.25
			envelope.Resonance.Readout = []float64{1}
			output = [3]*data.Measurement[float64]{}
			So(node.project(envelope, output[:]), ShouldBeNil)
			So(output[0], ShouldEqual, prior)
			_, present := output[0].Metrics["EntropyBits"]
			So(present, ShouldBeFalse)
			_, present = output[1].Metrics["readout.1"]
			So(present, ShouldBeFalse)
			So(output[2], ShouldBeNil)
			So(node.Grid.Step(output[:]), ShouldBeNil)
			So(output[0].Metrics["Confidence"].Coordinates, ShouldEqual, coordinates)
			So(output[0].Metrics["Confidence"].Raw, ShouldEqual, 0.25)
		})

		Convey("A failed solver readout cannot be interpreted as valid numerical evidence", func() {
			envelope.Cognition.Error = "source failed"
			So(node.project(envelope, output[:]), ShouldNotBeNil)
		})
	})
}

func BenchmarkGridNodeProject(b *testing.B) {
	node := &gridNode{Grid: learning.NewGrid()}
	envelope := types.NewEnvelope(types.EnvelopeTicker)
	envelope.TickerData.Symbol = "TEST/USD"
	envelope.Cognition = &types.Cognition{Confidence: 0.75, Cohort: 100}
	envelope.Resonance = &types.ResonanceArtifact{Readout: []float64{1, 2, 3, 4}, ForwardCurve: []float64{1, 2, 3}, Confidence: 0.5}
	envelope.Manifold = &types.ManifoldState{Version: 1, Reading: sensorium.Reading{Divergence: 2, KuramotoR: 0.5}}
	output := [3]*data.Measurement[float64]{}
	b.ReportAllocs()

	for b.Loop() {
		envelope.Manifold.Version++
		if err := node.project(envelope, output[:]); err != nil {
			b.Fatal(err)
		}
	}
}
