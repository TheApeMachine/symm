package regulator

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

func TestNewSolver(t *testing.T) {
	Convey("Given a system configuration and context", t, func() {
		cfg := system.NewConfig()
		solver := NewSolver(t.Context(), nil, nil)

		Convey("It should instantiate a valid regulator solver", func() {
			So(solver, ShouldNotBeNil)
			So(solver.config, ShouldEqual, cfg)
			So(solver.Close(), ShouldBeNil)
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given an active regulator solver and thesis", t, func() {
		cfg := system.NewConfig()
		solver := NewSolver(t.Context(), nil, nil)
		defer solver.Close()
		thesis := types.NewThesis(t.Context(), nil)

		initialAlpha := cfg.Resonance.LearningRate
		err := solver.Update(thesis)

		Convey("It should settle metrics and tune system config", func() {
			So(err, ShouldBeNil)
			So(cfg.Resonance.LearningRate, ShouldBeGreaterThan, 0)
			So(cfg.Manifold.RelaxationSteps, ShouldBeGreaterThan, 0)
			So(cfg.Risk.UncertaintyScale, ShouldBeGreaterThan, 0)
			So(cfg.Planner.MaxAllocationFraction, ShouldBeGreaterThan, 0)
			So(initialAlpha, ShouldBeGreaterThan, 0)
		})

		Convey("It should construct a visual RegulatorPayload", func() {
			initialPayload := solver.buildPayload(0.0, 0.0, 0.0)
			So(initialPayload.Status, ShouldEqual, "observing")
			So(initialPayload.Subsystems, ShouldHaveLength, 6)

			for range 5 {
				solver.recordHistory(0.2)
			}

			solver.coder.SetStreamLearn(true)
			payload := solver.buildPayload(0.2, 0.1, 0.0)
			So(payload.Subsystems, ShouldHaveLength, 6)
		})
	})
}

func BenchmarkUpdate(b *testing.B) {
	ctx := b.Context()
	solver := NewSolver(ctx, nil, nil)
	defer solver.Close()
	thesis := types.NewThesis(ctx, nil)

	for b.Loop() {
		_ = solver.Update(thesis)
	}
}
