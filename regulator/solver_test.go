package regulator

import (
	"math"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

func TestNewSolver(t *testing.T) {
	Convey("Given a complete optimizer configuration", t, func() {
		system.Cfg = system.NewConfig()
		solver, err := NewSolver(t.Context(), nil)

		Convey("It should construct the predictive model and fixed control space", func() {
			So(err, ShouldBeNil)
			So(solver, ShouldNotBeNil)
			So(solver.Status(), ShouldEqual, types.READY)
			So(solver.optimizer.coder, ShouldNotBeNil)
			So(solver.optimizer.pending, ShouldBeNil)
			So(solver.Close(), ShouldBeNil)
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given a baseline valuation followed by a changed equity outcome", t, func() {
		system.Cfg = system.NewConfig()
		baseline := system.Cfg.Snapshot()
		thesis := types.NewThesis(t.Context(), nil)
		ui := make(chan []byte, 2)
		solver, err := NewSolver(t.Context(), ui)
		So(err, ShouldBeNil)
		defer solver.Close()
		So(appendEquity(thesis, 200), ShouldBeNil)

		err = solver.Update(thesis, true)
		firstPending := append([]float64(nil), solver.optimizer.pending...)
		firstConfig := system.Cfg.Snapshot()
		So(appendEquity(thesis, 180), ShouldBeNil)
		errAfterOutcome := solver.Update(thesis, true)
		regulated := system.Cfg.Snapshot()

		Convey("It should resolve the prior controls against only the later log return", func() {
			So(err, ShouldBeNil)
			So(errAfterOutcome, ShouldBeNil)
			So(firstPending, ShouldHaveLength, regulatorContextCount+controlCount)
			So(firstPending[0], ShouldEqual, 0.0)
			So(solver.optimizer.resolved, ShouldEqual, 1)
			So(firstConfig.Planner, ShouldResemble, baseline.Planner)
			So(regulated.Planner.MaxAllocationFraction,
				ShouldBeLessThan, baseline.Planner.MaxAllocationFraction)
			So(solver.lastEquity, ShouldEqual, 180.0)
			So(solver.peakEquity, ShouldEqual, 200.0)
			So(solver.history, ShouldHaveLength, 2)
			So(len(ui), ShouldEqual, 2)
		})
	})

	Convey("Given a later broker valuation with identical equity", t, func() {
		system.Cfg = system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		solver, err := NewSolver(t.Context(), nil)
		So(err, ShouldBeNil)
		defer solver.Close()
		So(appendEquity(thesis, 200), ShouldBeNil)
		So(solver.Update(thesis, true), ShouldBeNil)
		So(appendEquity(thesis, 200), ShouldBeNil)

		err = solver.Update(thesis, true)

		Convey("It should learn the economically real zero-return interval", func() {
			So(err, ShouldBeNil)
			So(solver.optimizer.resolved, ShouldEqual, 1)
			So(solver.history, ShouldHaveLength, 2)
		})
	})

	Convey("Given repeated valuations while the account has no exposure", t, func() {
		system.Cfg = system.NewConfig()
		baseline := system.Cfg.Snapshot()
		thesis := types.NewThesis(t.Context(), nil)
		solver, err := NewSolver(t.Context(), nil)
		So(err, ShouldBeNil)
		defer solver.Close()
		So(appendEquity(thesis, 200), ShouldBeNil)
		So(solver.Update(thesis, false), ShouldBeNil)
		So(appendEquity(thesis, 200), ShouldBeNil)

		err = solver.Update(thesis, false)

		Convey("It should retain the configured controls without fitting a response", func() {
			So(err, ShouldBeNil)
			So(solver.optimizer.pending, ShouldBeNil)
			So(solver.optimizer.resolved, ShouldEqual, 0)
			So(system.Cfg.Snapshot().Planner, ShouldResemble, baseline.Planner)
		})
	})

	Convey("Given concurrent delivery of one changed equity revision", t, func() {
		system.Cfg = system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		solver, err := NewSolver(t.Context(), nil)
		So(err, ShouldBeNil)
		defer solver.Close()
		So(appendEquity(thesis, 200), ShouldBeNil)
		So(solver.Update(thesis, true), ShouldBeNil)
		So(appendEquity(thesis, 201), ShouldBeNil)
		var updates sync.WaitGroup
		errs := make(chan error, 16)

		for range 16 {
			updates.Go(func() {
				errs <- solver.Update(thesis, true)
			})
		}

		updates.Wait()
		close(errs)

		Convey("It should spend the revision exactly once", func() {
			for err := range errs {
				So(err, ShouldBeNil)
			}

			So(solver.optimizer.resolved, ShouldEqual, 1)
			So(solver.history, ShouldHaveLength, 2)
		})
	})
}

func TestFinancialFeedback(t *testing.T) {
	Convey("Given a drawdown followed by a partial recovery", t, func() {
		solver := &Solver{lastEquity: 200, peakEquity: 200}

		loss, drawdown := solver.financialFeedback(180)
		solver.lastEquity = 180
		recovery, recoveryDrawdown := solver.financialFeedback(198)

		Convey("It should preserve additive return and peak-relative drawdown separately", func() {
			So(loss, ShouldAlmostEqual, math.Log(0.9), 1e-12)
			So(drawdown, ShouldAlmostEqual, math.Log(0.9), 1e-12)
			So(recovery, ShouldAlmostEqual, math.Log(1.1), 1e-12)
			So(recoveryDrawdown, ShouldAlmostEqual, math.Log(0.99), 1e-12)
		})
	})
}

func TestRecordHistory(t *testing.T) {
	Convey("Given more reconstruction errors than the configured UI capacity", t, func() {
		solver := &Solver{historyCapacity: 3, history: make([]float64, 0, 3)}

		for reading := range 5 {
			solver.recordHistory(float64(reading))
		}

		Convey("It should retain the newest bounded observations", func() {
			So(solver.history, ShouldResemble, []float64{2, 3, 4})
		})
	})
}

func BenchmarkUpdate(b *testing.B) {
	system.Cfg = system.NewConfig()
	thesis := types.NewThesis(b.Context(), nil)
	solver, err := NewSolver(b.Context(), nil)

	if err != nil {
		b.Fatal(err)
	}

	defer solver.Close()
	value := 200.0

	for b.Loop() {
		value += 0.01

		if err := appendEquity(thesis, value); err != nil {
			b.Fatal(err)
		}

		if err := solver.Update(thesis, true); err != nil {
			b.Fatal(err)
		}
	}
}

func appendEquity(thesis *types.Thesis, value float64) error {
	return thesis.AppendEquity(kraken.TradeBalanceResult{
		Equity: decimal.NewFromFloat64(value),
	})
}
