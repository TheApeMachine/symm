package regulator

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

func TestNewSolver(t *testing.T) {
	Convey("Given a system configuration and context", t, func() {
		system.Cfg = system.NewConfig()
		solver := NewSolver(t.Context(), nil)

		Convey("It should instantiate a valid regulator solver", func() {
			So(solver, ShouldNotBeNil)
			So(solver.configSource, ShouldEqual, system.Cfg)
			So(solver.config == solver.configSource, ShouldBeFalse)
			So(solver.config.Planner == solver.configSource.Planner, ShouldBeFalse)
			So(solver.config.Planner, ShouldResemble, solver.configSource.Planner)
			_, err := solver.coder.SettleFromBatch(
				make([]float64, regulatorMetricCount), []float64{0},
			)
			So(err, ShouldBeNil)
			_, err = solver.coder.SettleFromBatch(make([]float64, 16), []float64{0})
			So(err, ShouldNotBeNil)
			So(solver.Close(), ShouldBeNil)
		})
	})
}

func TestRun(t *testing.T) {
	Convey("Given direct equity feedback", t, func() {
		system.Cfg = system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)
		ui := make(chan []byte, 1)
		solver := NewSolver(t.Context(), ui)
		defer solver.Close()

		So(solver.Update(thesis), ShouldBeNil)

		Convey("Then the regulator should settle and publish", func() {
			So(<-ui, ShouldNotBeEmpty)
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

		baselineReturn, baselineDrawdown, baselineReady := solver.readFinancialFeedback(thesis)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(180),
		}), ShouldBeNil)
		drawdownReturn, drawdown, drawdownReady := solver.readFinancialFeedback(thesis)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(198),
		}), ShouldBeNil)
		recoveryReturn, recoveryDrawdown, recoveryReady := solver.readFinancialFeedback(thesis)

		Convey("It should retain period return and drawdown from peak independently", func() {
			So(baselineReady, ShouldBeTrue)
			So(baselineReturn, ShouldEqual, 0.0)
			So(baselineDrawdown, ShouldEqual, 0.0)
			So(drawdownReady, ShouldBeTrue)
			So(drawdownReturn, ShouldAlmostEqual, -0.1, 1e-12)
			So(drawdown, ShouldAlmostEqual, -0.1, 1e-12)
			So(recoveryReady, ShouldBeTrue)
			So(recoveryReturn, ShouldAlmostEqual, 0.1, 1e-12)
			So(recoveryDrawdown, ShouldAlmostEqual, -0.01, 1e-12)
		})
	})

	Convey("Given no account valuation", t, func() {
		periodReturn, drawdown, ready := (&Solver{}).readFinancialFeedback(
			types.NewThesis(t.Context(), nil),
		)

		Convey("It should freeze instead of treating absent equity as zero", func() {
			So(ready, ShouldBeFalse)
			So(periodReturn, ShouldEqual, 0.0)
			So(drawdown, ShouldEqual, 0.0)
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given an active regulator solver and thesis", t, func() {
		system.Cfg = system.NewConfig()
		cfg := system.Cfg
		thesis := types.NewThesis(t.Context(), nil)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)
		solver := NewSolver(t.Context(), nil)
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
			So(solver.pace.Count(), ShouldEqual, 1)
			So(cfg.Resonance.LearningRate, ShouldEqual, solver.pace.Alpha())
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

	Convey("Given concurrent account feedback updates", t, func() {
		system.Cfg = system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)
		solver := NewSolver(t.Context(), nil)
		defer solver.Close()
		var updates sync.WaitGroup
		errs := make(chan error, 16)

		for range 16 {
			updates.Go(func() {
				errs <- solver.Update(thesis)
			})
		}

		updates.Wait()
		close(errs)

		Convey("It should serialize manifold and state mutation", func() {
			for err := range errs {
				So(err, ShouldBeNil)
			}

			So(solver.pace.Count(), ShouldEqual, 16)
			So(solver.history, ShouldHaveLength, 16)
		})
	})

	Convey("Given a calibrated quiet history followed by an equity shock", t, func() {
		system.Cfg = system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(200),
		}), ShouldBeNil)
		solver := NewSolver(t.Context(), nil)
		defer solver.Close()
		solver.pace = learning.NewPaceController(learning.PaceConfig{
			InitialAlpha: solver.learningRate,
			Window:       4,
		})

		for range 4 {
			So(solver.Update(thesis), ShouldBeNil)
		}

		initialAlpha := solver.learningRate
		So(thesis.AppendEquity(kraken.TradeBalanceResult{
			Equity: decimal.NewFromInt64(100),
		}), ShouldBeNil)
		So(solver.Update(thesis), ShouldBeNil)

		Convey("It should publish the empirically adapted manifold pace", func() {
			So(solver.rankReady, ShouldBeTrue)
			So(solver.config.Resonance.LearningRate, ShouldBeGreaterThan, initialAlpha)
			So(system.Cfg.Snapshot().Resonance.LearningRate,
				ShouldEqual, solver.config.Resonance.LearningRate)
		})
	})
}

func TestRecordHistory(t *testing.T) {
	Convey("Given more readings than the empirical pace horizon", t, func() {
		system.Cfg = system.NewConfig()
		solver := NewSolver(t.Context(), nil)
		defer solver.Close()

		for reading := range 300 {
			_, err := solver.pace.Measure(float64(reading))
			So(err, ShouldBeNil)
			solver.recordHistory(float64(reading))
		}

		Convey("It should retain the same rolling horizon as the calibrator", func() {
			So(solver.history, ShouldHaveLength, solver.pace.Count())
			So(solver.history[0], ShouldEqual, 44.0)
			So(solver.history[len(solver.history)-1], ShouldEqual, 299.0)
		})
	})
}

func TestApplyTuning(t *testing.T) {
	Convey("Given a calibrated high-surprise rank", t, func() {
		system.Cfg = system.NewConfig()
		solver := NewSolver(t.Context(), nil)
		defer solver.Close()
		window := 2
		solver.pace = learning.NewPaceController(learning.PaceConfig{
			InitialAlpha: solver.learningRate,
			Window:       window,
		})

		for range window {
			_, err := solver.pace.Measure(0)
			So(err, ShouldBeNil)
		}

		pace, err := solver.pace.Measure(1)
		So(err, ShouldBeNil)
		solver.rankReady = pace.Ready
		solver.surpriseRank = pace.Rank

		err = solver.applyTuning()

		Convey("It should increase causal scrutiny without expanding search latency", func() {
			So(err, ShouldBeNil)
			So(solver.config.Planner.CausalAlpha, ShouldBeGreaterThan, solver.causalAlpha)
			So(solver.config.Planner.MCTSIterations, ShouldBeLessThanOrEqualTo, solver.iterations)
			So(solver.config.Planner.MCTSIterations, ShouldBeGreaterThan, 0)
		})
	})
}

func TestFormatFloat(t *testing.T) {
	Convey("Given financial values with leading fractional zeroes and a negative fraction", t, func() {
		Convey("It should preserve exact fixed-point presentation", func() {
			So(formatFloat(1.005, 3), ShouldEqual, "1.005")
			So(formatFloat(-0.25, 2), ShouldEqual, "-0.25")
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

	solver := NewSolver(ctx, nil)
	defer solver.Close()

	for b.Loop() {
		_ = solver.Update(thesis)
	}
}
