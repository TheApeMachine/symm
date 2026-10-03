package types

import (
	"testing"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func manifoldWireFixture() *ManifoldState {
	return &ManifoldState{
		At: time.Unix(100, 123), Version: 7,
		State: &sensorium.State{N: 2, ContentIDs: []int64{41, 42}, Phase: []float32{.5, 1}, Pos: []float32{0, .1, .2, .3, .4, .5}, Mass: []float32{1, 2}},
		GridX: 64, GridY: 64, GridZ: 64, GridSpacing: 1.0 / 64,
		MomRho: make([]float32, 64*64*64*4), FieldEnergy: make([]float32, 64*64*64), WaveReal: make([]float32, 64*64*64), WaveImag: make([]float32, 64*64*64),
		Modes:      []WaveMode{{Omega: 2, Real: .3, Imag: -.4, Linewidth: .5}},
		Resultants: []PhaseChannelResultant{{Side: "bid", Count: 1, TotalAmplitude: 2, Coherence: .8, Phase: .4}},
		Reading: sensorium.Reading{GuidanceSpeed: 2, Health: sensorium.PhysicsHealth{
			Integrator: sensorium.IntegratorHealth{AcceptedDT: .002, Time: .1, Substeps: 3, Rejections: 2},
			Gas:        sensorium.GasHealth{Mass: 3, Momentum: [3]float64{1, 2, 3}, MaxMach: 4},
			Wave:       sensorium.WaveHealth{Norm: 5, ProjectedNorm: 6}, Pilot: sensorium.PilotHealth{MinDensity: .001},
			Sources: sensorium.SourceLedger{PICDepositEnergyResidual: -.0001}, ParticleMaterialTotal: 7,
		}},
	}
}

func TestEncodeManifold(t *testing.T) {
	Convey("The published frame preserves particles, fields, modes and accepted physics health", t, func() {
		state := manifoldWireFixture()
		encoded, err := EncodeManifold(state)
		So(err, ShouldBeNil)
		message := wire.GetRootAsMessage(encoded, 0)
		So(message.FrameType(), ShouldEqual, wire.FrameManifoldFrame)
		var table flatbuffers.Table
		So(message.Frame(&table), ShouldBeTrue)
		frame := &wire.ManifoldFrame{}
		frame.Init(table.Bytes, table.Pos)
		decoded := frame.UnPack()
		So(decoded.Version, ShouldEqual, 7)
		So(decoded.ContentIds, ShouldResemble, state.State.ContentIDs)
		So(decoded.Pos, ShouldResemble, state.State.Pos)
		So(decoded.MomRho, ShouldResemble, state.MomRho)
		So(decoded.Modes[0].Imaginary, ShouldEqual, float32(-.4))
		So(decoded.Resultants[0].Side, ShouldEqual, "bid")
		So(decoded.Reading.Health.Integrator.AcceptedDt, ShouldEqual, .002)
		So(decoded.Reading.Health.Integrator.Rejections, ShouldEqual, 2)
		So(decoded.Reading.Health.Gas.Momentum, ShouldResemble, []float64{1, 2, 3})
		So(decoded.Reading.Health.Wave.ProjectedNorm, ShouldEqual, 6)
		So(decoded.Reading.Health.Pilot.MinDensity, ShouldEqual, .001)
		So(decoded.Reading.Health.Sources.PicDepositEnergyResidual, ShouldEqual, -.0001)
		Convey("Reusing the builder does not overwrite an earlier frame", func() {
			state.Version++
			state.Reading.Health.Integrator.Time = .2
			_, err := EncodeManifold(state)
			So(err, ShouldBeNil)
			So(frame.Version(), ShouldEqual, 7)
			So(frame.Reading(nil).Health(nil).Integrator(nil).Time(), ShouldEqual, .1)
		})
	})
}

func BenchmarkEncodeManifold(b *testing.B) {
	state := manifoldWireFixture()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := EncodeManifold(state); err != nil {
			b.Fatal(err)
		}
	}
}
