package cmd

import (
	"math"
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
		So(output[2].Metrics, ShouldHaveLength, 7)
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

func TestGridNodeManifold(t *testing.T) {
	Convey("Given evolving bid/ask particles and complex spectral modes", t, func() {
		node := &gridNode{Grid: learning.NewGrid()}
		envelope := types.NewEnvelope(types.EnvelopeTicker)
		envelope.TickerData.Symbol = "TEST/USD"
		envelope.Manifold = &types.ManifoldState{
			Version: 1, At: time.Unix(1, 0),
			State: sensorium.State{N: 2, TokenIDs: []int64{2, 3},
				Energy: []float32{4, 9}, Heat: []float32{2, 6},
				Vel: []float32{1, -2, 3, 3, 4, -1}},
			Modes: []types.WaveMode{{Real: 3, Imag: 4}, {Real: 0, Imag: -2}},
		}
		So(node.Step(envelope), ShouldEqual, envelope)
		So(node.Error(), ShouldBeNil)
		expected := map[string]float64{
			"wavespace.power.0": 25, "wavespace.power.1": 4,
			"wavespace.phase.0": math.Atan2(4, 3), "wavespace.phase.1": -math.Pi / 2,
			"particles.count": 2, "particles.energy.bid": 4, "particles.energy.ask": 9,
			"particles.heat.bid": 2, "particles.heat.ask": 6,
			"particles.vel.mean_x": 2, "particles.vel.mean_y": 1, "particles.vel.mean_z": 1,
		}

		for label, value := range expected {
			column := node.Grid.Column("manifold", label)
			So(column, ShouldBeGreaterThanOrEqualTo, 0)
			So(node.Grid.Values[0][column], ShouldAlmostEqual, value)
			So(node.Grid.Present[0][column], ShouldBeTrue)
		}

		Convey("Repeated envelopes do not train again on the same physics version", func() {
			version := node.Grid.Version
			So(node.Step(envelope), ShouldEqual, envelope)
			So(node.Error(), ShouldBeNil)
			So(node.Grid.Version, ShouldEqual, version)
		})

		Convey("A later version changes the actual learning values and removes absent components", func() {
			envelope.Manifold.Version++
			envelope.Manifold.Modes = []types.WaveMode{{Real: -2}}
			envelope.Manifold.Energy = []float32{10, 1}
			So(node.Step(envelope), ShouldEqual, envelope)
			So(node.Error(), ShouldBeNil)
			So(node.Grid.Values[0][node.Grid.Column("manifold", "wavespace.power.0")], ShouldEqual, 4)
			So(node.Grid.Values[0][node.Grid.Column("manifold", "wavespace.phase.0")], ShouldAlmostEqual, math.Pi)
			So(node.Grid.Values[0][node.Grid.Column("manifold", "particles.energy.bid")], ShouldEqual, 10)
			So(node.Grid.Present[0][node.Grid.Column("manifold", "wavespace.power.1")], ShouldBeFalse)

			Convey("An empty population publishes its count without inventing a mean velocity", func() {
				envelope.Manifold.Version++
				envelope.Manifold.State = sensorium.State{}
				So(node.Step(envelope), ShouldEqual, envelope)
				So(node.Error(), ShouldBeNil)
				So(node.Grid.Values[0][node.Grid.Column("manifold", "particles.count")], ShouldEqual, 0)
				So(node.Grid.Present[0][node.Grid.Column("manifold", "particles.vel.mean_x")], ShouldBeFalse)
			})
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
	// One full 100-level book on each side and a 64-bin spectrum exercise
	// the population scan and reusable spectral projection storage.
	envelope.Manifold.State = sensorium.State{N: 200, TokenIDs: make([]int64, 200),
		Energy: make([]float32, 200), Heat: make([]float32, 200), Vel: make([]float32, 600)}
	envelope.Manifold.Modes = make([]types.WaveMode, 64)

	for index := range envelope.Manifold.TokenIDs {
		envelope.Manifold.TokenIDs[index] = int64(index)
		envelope.Manifold.Energy[index] = float32(index + 1)
		envelope.Manifold.Heat[index] = float32(index) / 2
	}

	for index := range envelope.Manifold.Modes {
		envelope.Manifold.Modes[index] = types.WaveMode{Real: float32(index), Imag: 1}
	}

	output := [3]*data.Measurement[float64]{}
	b.ReportAllocs()

	for b.Loop() {
		envelope.Manifold.Version++
		if err := node.project(envelope, output[:]); err != nil {
			b.Fatal(err)
		}
	}
}
