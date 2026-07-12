package manifold

import (
	"testing"
	"time"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReplayRecorder(t *testing.T) {
	Convey("Given a lock-free replay recorder", t, func() {
		recorder := NewReplayRecorder()

		pushed := recorder.Record(
			"BTC/USD",
			ObservationMetadata{
				At: time.Unix(1, 0), FrameType: "update", Checksum: 42, Count: 3,
			},
			State{
				At:           time.Unix(1, 0),
				Epoch:        1,
				Ready:        true,
				VisibleMass:  5,
				DeltaT:       0.1,
				Subdivisions: 1,
				PriceScale:   0.01,
				SizeScale:    0.5,
				Reading: pmanifold.Reading{
					PressureGradNorm: 0.1,
					Divergence:       0.2,
					CoherenceMag2:    0.3,
					GuidanceSpeed:    0.4,
					ViscosityProxy:   0.5,
				},
			},
			PopulationAccounting{Initial: 5},
			1,
			2,
			1,
		)

		Convey("It should snapshot frames without draining the ring", func() {
			So(pushed, ShouldBeTrue)
			first := recorder.Frames()
			second := recorder.Frames()

			So(first, ShouldHaveLength, 1)
			So(second, ShouldHaveLength, 1)
			So(first[0].Symbol, ShouldEqual, "BTC/USD")
			So(first[0].Epoch, ShouldEqual, 1)
			So(first[0].VisibleMass, ShouldEqual, 5)
			So(first[0].DeltaT, ShouldEqual, 0.1)
			So(first[0].DepositCount, ShouldEqual, 1)
			So(first[0].Checksum, ShouldEqual, 42)
			So(first[0].Observations, ShouldEqual, 3)
		})
	})
}

func BenchmarkReplayRecorderRecord(b *testing.B) {
	recorder := NewReplayRecorder()
	state := State{
		At:           time.Unix(1, 0),
		Ready:        true,
		DeltaT:       0.1,
		Subdivisions: 1,
		PriceScale:   0.01,
		SizeScale:    0.5,
		Reading: pmanifold.Reading{
			PressureGradNorm: 0.1,
			Divergence:       0.2,
			CoherenceMag2:    0.3,
			GuidanceSpeed:    0.4,
			ViscosityProxy:   0.5,
		},
	}

	for b.Loop() {
		recorder.Record(
			"BTC/USD",
			ObservationMetadata{At: time.Unix(1, 0), Count: 1},
			state,
			PopulationAccounting{Initial: 1},
			1,
			1,
			1,
		)
	}
}
