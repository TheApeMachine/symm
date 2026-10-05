package ui

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

func manifoldFixture() *types.ManifoldState {
	return &types.ManifoldState{
		At:      time.Unix(100, 123),
		Version: 7,
		State: &sensorium.State{
			N:          2,
			ContentIDs: []int64{41, 42},
			Phase:      []float32{0.5, 1},
			Pos:        []float32{0, 0.1, 0.2, 0.3, 0.4, 0.5},
			Mass:       []float32{1, 2},
		},
		GridX:       64,
		GridY:       64,
		GridZ:       64,
		GridSpacing: 1.0 / 64,
		MomRho:      make([]float32, 64*64*64*4),
		FieldEnergy: make([]float32, 64*64*64),
		WaveReal:    make([]float32, 64*64*64),
		WaveImag:    make([]float32, 64*64*64),
		Modes:       []types.WaveMode{{Omega: 2, Real: 0.3, Imag: -0.4, Linewidth: 0.5}},
		Resultants:  []types.PhaseChannelResultant{{Side: "bid", Count: 1, TotalAmplitude: 2, Coherence: 0.8, Phase: 0.4}},
		Reading: sensorium.Reading{
			GuidanceSpeed: 2,
			Health: sensorium.PhysicsHealth{
				Integrator:            sensorium.IntegratorHealth{AcceptedDT: 0.002, Time: 0.1, Substeps: 3, Rejections: 2},
				Gas:                   sensorium.GasHealth{Mass: 3, Momentum: [3]float64{1, 2, 3}, MaxMach: 4},
				Wave:                  sensorium.WaveHealth{Norm: 5, ProjectedNorm: 6},
				Pilot:                 sensorium.PilotHealth{MinDensity: 0.001},
				Sources:               sensorium.SourceLedger{PICDepositEnergyResidual: -0.0001},
				ParticleMaterialTotal: 7,
			},
		},
	}
}

/*
awaitFrame returns the next frame encoded by the tee's background worker,
failing the test when the worker produces nothing within the deadline.
*/
func awaitFrame(t *testing.T, tee *UITee) []byte {
	deadline := time.After(time.Second)

	for {
		if frame := tee.Next(); frame != nil {
			return *(*[]byte)(frame)
		}

		select {
		case <-tee.Available():
		case <-deadline:
			t.Fatal("timed out waiting for the tee worker to encode a frame")
		}
	}
}

func sentinel(seq int64) *data.Measurement[float64] {
	measurement := data.NewMeasurement[float64]("hawkes:trade", nil)
	measurement.Label = "BTC/USD"
	measurement.SeqIdx = seq

	return measurement
}

func TestUITeeNext(t *testing.T) {
	Convey("Queued websocket metrics yield frames immediately without batching", t, func() {
		originalRoute, originalFocus := types.Route(), types.Focus()
		defer types.SetRoute(originalRoute)
		defer types.SetFocus(originalFocus)
		types.SetRoute("dashboard")
		types.SetFocus("BTC/USD")
		tee := NewUITee(t.Context(), "route-test")
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()

		Convey("An unready tee returns nil", func() {
			unreadyTee := NewUITee(t.Context(), "unready-test")
			defer func() { So(unreadyTee.Close(), ShouldBeNil) }()
			So(unreadyTee.Next() == nil, ShouldBeTrue)
		})

		Convey("An empty queue returns nil", func() {
			So(tee.Next() == nil, ShouldBeTrue)
		})

		Convey("Pushed measurement is yielded as a single-measurement frame", func() {
			tee.Push(data.Publication{Measurement: sentinel(42)})

			decoded := wire.GetRootAsMeasurementsFrame(awaitFrame(t, tee), 0).UnPack()
			So(len(decoded.Rows), ShouldEqual, 1)
			So(decoded.Rows[0].Tick, ShouldEqual, 42)
			So(decoded.Rows[0].Symbol, ShouldEqual, "BTC/USD")

			So(tee.Next() == nil, ShouldBeTrue)
		})

		Convey("Manifold state measurement is yielded as ManifoldFrame", func() {
			manifoldMeasurement := data.NewMeasurement[float64]("hawkes:trade", nil)
			manifoldMeasurement.Label = "BTC/USD"
			manifoldMeasurement.Result = manifoldFixture()
			tee.Push(data.Publication{Measurement: manifoldMeasurement})

			message := wire.GetRootAsMessage(awaitFrame(t, tee), 0)
			So(message.FrameType(), ShouldEqual, wire.FrameManifoldFrame)

			So(tee.Next() == nil, ShouldBeTrue)
		})

		Convey("Duplicate manifold versions are dropped", func() {
			fixture := manifoldFixture()
			fixture.Version = 10
			manifoldMeasurement := data.NewMeasurement[float64]("hawkes:trade", nil)
			manifoldMeasurement.Label = "BTC/USD"
			manifoldMeasurement.Result = fixture
			tee.Push(data.Publication{Measurement: manifoldMeasurement})

			first := wire.GetRootAsMessage(awaitFrame(t, tee), 0)
			So(first.FrameType(), ShouldEqual, wire.FrameManifoldFrame)

			// The duplicate is dropped, so the sentinel pushed after it is next.
			tee.Push(data.Publication{Measurement: manifoldMeasurement})
			tee.Push(data.Publication{Measurement: sentinel(43)})

			decoded := wire.GetRootAsMeasurementsFrame(awaitFrame(t, tee), 0).UnPack()
			So(decoded.Rows[0].Tick, ShouldEqual, 43)

			newFixture := manifoldFixture()
			newFixture.Version = 11
			manifoldMeasurement2 := data.NewMeasurement[float64]("hawkes:trade", nil)
			manifoldMeasurement2.Label = "BTC/USD"
			manifoldMeasurement2.Result = newFixture
			tee.Push(data.Publication{Measurement: manifoldMeasurement2})

			second := wire.GetRootAsMessage(awaitFrame(t, tee), 0)
			So(second.FrameType(), ShouldEqual, wire.FrameManifoldFrame)
		})

		Convey("Frames are yielded in push order", func() {
			for seq := int64(1); seq <= 64; seq++ {
				tee.Push(data.Publication{Measurement: sentinel(seq)})
			}

			var seen int64
			for seen < 64 {
				decoded := wire.GetRootAsMeasurementsFrame(awaitFrame(t, tee), 0).UnPack()
				for _, row := range decoded.Rows {
					seen++
					So(row.Tick, ShouldEqual, seen)
				}
			}
		})
	})
}

func BenchmarkUITeeNext(b *testing.B) {
	originalRoute, originalFocus := types.Route(), types.Focus()
	defer types.SetRoute(originalRoute)
	defer types.SetFocus(originalFocus)
	types.SetRoute("xray")
	types.SetFocus("BTC/USD")
	tee := NewUITee(b.Context(), "benchmark")
	tee.Transition(runtime.READY)
	defer func() {
		if err := tee.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	measurement := data.NewMeasurement[float64]("hawkes:trade", nil)
	measurement.Label = "BTC/USD"
	measurement.SetMetric("conditional_intensity", data.Metric[float64]{Label: "conditional_intensity", Raw: 1.2})
	measurement.SetMetric("background_rate", data.Metric[float64]{Label: "background_rate", Raw: 0.5})

	for b.Loop() {
		tee.Push(data.Publication{Measurement: measurement})
		tee.Next()
	}
}

func TestUITeePush(t *testing.T) {
	Convey("Push queues measurements correctly", t, func() {
		originalRoute, originalFocus := types.Route(), types.Focus()
		defer types.SetRoute(originalRoute)
		defer types.SetFocus(originalFocus)
		types.SetFocus("BTC/USD")
		types.SetRoute("learning")
		tee := NewUITee(t.Context(), "push-test")
		tee.Transition(runtime.READY)
		defer func() { So(tee.Close(), ShouldBeNil) }()

		measurement := data.NewMeasurement[float64]("training", nil)
		measurement.Label, measurement.SeqIdx = "BTC/USD", 1

		Convey("A rejected route drops the measurement immediately", func() {
			types.SetRoute("journal")
			tee.Push(data.Publication{Measurement: measurement})
			types.SetRoute("learning")

			follower := data.NewMeasurement[float64]("training", nil)
			follower.Label, follower.SeqIdx = "BTC/USD", 2
			tee.Push(data.Publication{Measurement: follower})

			decoded := wire.GetRootAsMeasurementsFrame(awaitFrame(t, tee), 0).UnPack()
			So(decoded.Rows[0].Tick, ShouldEqual, 2)
		})

		Convey("An accepted route queues the measurement for serialization", func() {
			types.SetRoute("learning")
			tee.Push(data.Publication{Measurement: measurement})

			decoded := wire.GetRootAsMeasurementsFrame(awaitFrame(t, tee), 0).UnPack()
			So(len(decoded.Rows), ShouldEqual, 1)
			So(decoded.Rows[0].Tick, ShouldEqual, 1)
			So(decoded.Rows[0].Symbol, ShouldEqual, "BTC/USD")
		})
	})
}
