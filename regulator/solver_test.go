package regulator

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

func TestNewSolver(t *testing.T) {
	Convey("Given a system configuration and context", t, func() {
		cfg := system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		solver := NewSolver(t.Context(), nil, thesis)

		Convey("It should instantiate a valid regulator solver", func() {
			So(solver, ShouldNotBeNil)
			So(solver.config, ShouldEqual, cfg)
			So(solver.Close(), ShouldBeNil)
		})
	})
}

func TestRun(t *testing.T) {
	Convey("Given targeted equity feedback", t, func() {
		system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)
		ui := make(chan []byte, 1)
		solver := NewSolver(t.Context(), ui, thesis)
		defer solver.Close()

		thesis.Fanout(types.SourceEquity, types.SourceRegulator)

		Convey("Then the regulator should settle and publish without waking the pipeline", func() {
			select {
			case frame := <-ui:
				So(frame, ShouldNotBeEmpty)
			case <-time.After(time.Second):
				t.Fatal("regulator did not process targeted equity")
			}
		})
	})
}

func TestReadFinancialFeedback(t *testing.T) {
	Convey("Given successive complete equity valuations", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		solver := &Solver{}
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)

		baseline, baselineReady := solver.readFinancialFeedback(thesis)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(180),
		}), ShouldBeNil)
		drawdown, drawdownReady := solver.readFinancialFeedback(thesis)

		Convey("It should establish the baseline and measure later equity change", func() {
			So(baselineReady, ShouldBeTrue)
			So(baseline, ShouldEqual, 0.0)
			So(drawdownReady, ShouldBeTrue)
			So(drawdown, ShouldAlmostEqual, -0.1, 1e-12)
		})
	})

	Convey("Given no account valuation", t, func() {
		feedback, ready := (&Solver{}).readFinancialFeedback(
			types.NewThesis(t.Context(), nil),
		)

		Convey("It should freeze instead of treating absent equity as zero", func() {
			So(ready, ShouldBeFalse)
			So(feedback, ShouldEqual, 0.0)
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given an active regulator solver and thesis", t, func() {
		cfg := system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)
		solver := NewSolver(t.Context(), nil, thesis)
		defer solver.Close()

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
	thesis := types.NewThesis(ctx, nil)

	if err := thesis.AppendEquity(kraken.TradeBalanceResult{
		Equity: decimal.NewFromInt64(200),
	}); err != nil {
		b.Fatal(err)
	}

	solver := NewSolver(ctx, nil, thesis)
	defer solver.Close()

	for b.Loop() {
		_ = solver.Update(thesis)
	}
}
