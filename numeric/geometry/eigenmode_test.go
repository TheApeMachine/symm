package geometry

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEigenmodeMembers(t *testing.T) {
	t.Parallel()

	Convey("Members returns an independent copy of the mode list", t, func() {
		mode := &Eigenmode{
			members: []uint64{1, 2, 3},
			energy:  4.5,
		}

		copySlice := mode.Members()

		copySlice[0] = 99

		So(mode.members[0], ShouldEqual, 1)
		So(len(copySlice), ShouldEqual, 3)
	})
}

func TestEigenmodeEnergy(t *testing.T) {
	t.Parallel()

	Convey("Energy exposes the aggregate score", t, func() {
		mode := &Eigenmode{energy: 12.25}

		So(mode.Energy(), ShouldEqual, 12.25)
	})
}

func TestDetectModes(t *testing.T) {
	t.Parallel()

	Convey("Given no participants", t, func() {
		modes, dominant := DetectModes(nil, 0.5, func(a, b uint64) float64 { return 0 })

		So(len(modes), ShouldEqual, 0)
		So(dominant, ShouldEqual, -1)
	})

	Convey("Given participants with pairwise coupling at threshold", t, func() {
		participants := []ModeParticipant{
			{Origin: 10, Energy: 1},
			{Origin: 20, Energy: 2},
			{Origin: 30, Energy: 4},
		}

		couple := func(a, b uint64) float64 {
			if a == 10 && b == 20 || a == 20 && b == 10 {
				return 1
			}

			return 0
		}

		modes, dominant := DetectModes(participants, 1.0, couple)

		So(len(modes), ShouldEqual, 2)
		So(dominant, ShouldEqual, 1)
		So(modes[1].Energy(), ShouldEqual, 4)
		So(len(modes[0].members), ShouldEqual, 2)
	})
}

func TestPhaseModeFromDominant(t *testing.T) {
	t.Parallel()

	Convey("phaseModeFromDominant maps PhaseMode fields", t, func() {
		d := PhaseMode{
			Index:         3,
			Amplitude:     200,
			Concentration: 0.42,
		}

		mode := phaseModeFromDominant(d)

		So(mode.Index, ShouldEqual, 3)
		So(mode.Amplitude, ShouldEqual, 200)
		So(mode.Concentration, ShouldEqual, 0.42)
	})
}

func BenchmarkDetectModes(b *testing.B) {
	participants := make([]ModeParticipant, 64)

	for idx := range participants {
		participants[idx] = ModeParticipant{Origin: uint64(idx + 1), Energy: float64(idx%7 + 1)}
	}

	couple := func(a, b uint64) float64 {
		if (a+b)%3 == 0 {
			return 1
		}

		return 0
	}

	var modes []Eigenmode

	var dominant int

	b.ResetTimer()

	for b.Loop() {
		modes, dominant = DetectModes(participants, 0.7, couple)
	}

	_ = modes
	_ = dominant
}
