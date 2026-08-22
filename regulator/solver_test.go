package regulator

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/transport"
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
		consumer := transport.NewConsumer[*types.UIFrame]("test", func() {})
		ui := transport.NewMapReduce[*types.UIFrame](
			[]*transport.Consumer[*types.UIFrame]{consumer}, nil, nil,
		)
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
			So(int(ui.Length()), ShouldEqual, 2)
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
		thesis := types.NewThesis(t.Context(), nil)
		consumer := transport.NewConsumer[*types.UIFrame]("test", func() {})
		ui := transport.NewMapReduce[*types.UIFrame](
			[]*transport.Consumer[*types.UIFrame]{consumer}, nil, nil,
		)
		solver, err := NewSolver(t.Context(), ui)
		So(err, ShouldBeNil)
		defer solver.Close()
		So(appendEquity(thesis, 200), ShouldBeNil)
		So(solver.Update(thesis, false), ShouldBeNil)
		So(appendEquity(thesis, 200), ShouldBeNil)

		err = solver.Update(thesis, false)

		Convey("It should learn explicit inactivity instead of treating no trades as success", func() {
			So(err, ShouldBeNil)
			So(solver.optimizer.pending, ShouldHaveLength, regulatorContextCount+controlCount)
			So(solver.optimizer.resolved, ShouldEqual, 1)
			So(int(ui.Length()), ShouldEqual, 2)
		})
	})

	Convey("Given a zero allocation selected before the account becomes flat", t, func() {
		system.Cfg = system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		solver, err := NewSolver(t.Context(), nil)
		So(err, ShouldBeNil)
		defer solver.Close()
		zeroAllocation := solver.optimizer.current
		zeroAllocation[controlAllocation] = 0
		So(solver.applyControls(zeroAllocation), ShouldBeNil)
		solver.optimizer.current = zeroAllocation
		So(appendEquity(thesis, 200), ShouldBeNil)
		So(solver.Update(thesis, false), ShouldBeNil)
		So(appendEquity(thesis, 200), ShouldBeNil)

		err = solver.Update(thesis, false)

		Convey("It should identify zero allocation as inactive and explore away from it", func() {
			So(err, ShouldBeNil)
			So(solver.optimizer.current[controlAllocation],
				ShouldBeGreaterThan, 0)
			So(system.Cfg.Snapshot().Planner.MaxAllocationFraction,
				ShouldBeGreaterThan, 0)
			So(solver.optimizer.resolved, ShouldEqual, 1)
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

func TestObserveMark(t *testing.T) {
	Convey("Given executable marks between complete account valuations", t, func() {
		system.Cfg = system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		solver, err := NewSolver(t.Context(), nil)
		So(err, ShouldBeNil)
		defer solver.Close()
		So(appendEquity(thesis, 200), ShouldBeNil)
		So(solver.Update(thesis, true), ShouldBeNil)

		So(solver.ObserveMark(types.MarkFeedback{
			PositionID: "position-1", Symbol: "BTC/USD",
			At: time.Unix(1, 0).UTC(), Mark: 100,
			PeakDrawdown: 0, FloorDistance: 0.03, Exposed: true,
		}), ShouldBeNil)
		So(solver.ObserveMark(types.MarkFeedback{
			PositionID: "position-1", Symbol: "BTC/USD",
			At: time.Unix(2, 0).UTC(), Mark: 101,
			PeakDrawdown: math.Log(101.0 / 102.0), FloorDistance: 0.02,
			SurgeArmed: true, Exposed: true,
		}), ShouldBeNil)
		So(appendEquity(thesis, 201), ShouldBeNil)

		err = solver.Update(thesis, true)

		Convey("It should condition the next control state without counting marks as wallet outcomes", func() {
			So(err, ShouldBeNil)
			So(solver.markSamples, ShouldEqual, uint64(2))
			So(solver.lastMarkContext.samples, ShouldEqual, 2)
			So(solver.lastMarkContext.returnSamples, ShouldEqual, 1)
			So(solver.lastMarkContext.meanReturn, ShouldAlmostEqual, math.Log(1.01), 1e-12)
			So(solver.lastMarkContext.worstDrawdown, ShouldAlmostEqual, math.Log(101.0/102.0), 1e-12)
			So(solver.lastMarkContext.minimumFloor, ShouldAlmostEqual, 0.02, 1e-12)
			So(solver.lastMarkContext.surgeFraction, ShouldAlmostEqual, 0.5, 1e-12)
			So(solver.optimizer.resolved, ShouldEqual, 1)
			So(solver.optimizer.pending, ShouldHaveLength, regulatorContextCount+controlCount)
		})
	})
}


func TestObserveMarkPositionBoundary(t *testing.T) {
	Convey("Given a later position in the same symbol", t, func() {
		system.Cfg = system.NewConfig()
		solver, err := NewSolver(t.Context(), nil)
		So(err, ShouldBeNil)
		defer solver.Close()

		So(solver.ObserveMark(types.MarkFeedback{
			PositionID: "old-position", Symbol: "BTC/USD",
			At: time.Unix(1, 0).UTC(), Mark: 100, Exposed: true,
		}), ShouldBeNil)
		So(solver.ObserveMark(types.MarkFeedback{
			PositionID: "new-position", Symbol: "BTC/USD",
			At: time.Unix(2, 0).UTC(), Mark: 200, Exposed: true,
		}), ShouldBeNil)

		Convey("It should reset return continuity while keeping memory bounded by symbol", func() {
			context := solver.markAcc.snapshot()
			So(context.samples, ShouldEqual, 2)
			So(context.returnSamples, ShouldEqual, 0)
			So(solver.marks, ShouldHaveLength, 1)
			So(solver.marks["BTC/USD"].positionID, ShouldEqual, "new-position")
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

func TestObserveHindsight(t *testing.T) {

	Convey("Given hindsight attributions observed between complete account valuations", t, func() {
		system.Cfg = system.NewConfig()
		thesis := types.NewThesis(t.Context(), nil)
		solver, err := NewSolver(t.Context(), nil)
		So(err, ShouldBeNil)
		defer solver.Close()
		So(appendEquity(thesis, 200), ShouldBeNil)
		So(solver.Update(thesis, true), ShouldBeNil)

		So(solver.ObserveHindsight(types.HindsightFeedback{
			Symbol:         "BTC/USD",
			At:             time.Now(),
			Opportunity:    true,
			Captured:       true,
			RealizedReturn: 0.08,
		}), ShouldBeNil)

		So(solver.ObserveHindsight(types.HindsightFeedback{
			Symbol:          "ETH/USD",
			At:              time.Now(),
			Opportunity:     true,
			Missed:          true,
			MissedReturn:    0.12,
			DominantBlocker: "confidence",
		}), ShouldBeNil)

		So(appendEquity(thesis, 205), ShouldBeNil)
		err = solver.Update(thesis, true)

		Convey("It should condition the regulator context with delayed opportunity outcomes", func() {
			So(err, ShouldBeNil)
			So(solver.hindsightSamples, ShouldEqual, uint64(2))
			So(solver.lastHindsightContext.samples, ShouldEqual, 2)
			So(solver.lastHindsightContext.capturedSamples, ShouldEqual, 1)
			So(solver.lastHindsightContext.missedSamples, ShouldEqual, 1)
			So(solver.lastHindsightContext.meanCapturedReturn, ShouldAlmostEqual, 0.08, 1e-12)
			So(solver.lastHindsightContext.meanMissedReturn, ShouldAlmostEqual, 0.12, 1e-12)
			So(solver.lastHindsightContext.confidenceBlockCount, ShouldEqual, 1)
			So(solver.optimizer.resolved, ShouldEqual, 1)
			So(solver.optimizer.pending, ShouldHaveLength, regulatorContextCount+controlCount)
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
