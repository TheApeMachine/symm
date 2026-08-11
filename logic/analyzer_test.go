package logic

import (
	"errors"
	"slices"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/types"
)

/*
orderedSolver records when Analyzer invokes a solver.
*/
type orderedSolver struct {
	index  int
	order  *[]int
	mu     *sync.Mutex
	source types.SourceType
	err    error
}

func (solver *orderedSolver) Update(thesis *types.Thesis) error {
	solver.mu.Lock()
	*solver.order = append(*solver.order, solver.index)
	solver.mu.Unlock()

	if solver.err != nil {
		return solver.err
	}

	return nil
}

func (solver *orderedSolver) Close() error {
	return nil
}

func TestNewAnalyzer(t *testing.T) {
	Convey("Given a signal already subscribed under its actor identity", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		signal := make(chan struct{}, 1)
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		analyzer := NewAnalyzer(
			t.Context(), nil, nil, tree, nil, nil, nil, thesis,
		)
		defer analyzer.Close()

		Convey("Then the analyzer should not replace the signal subscription", func() {
			So(len(signal), ShouldEqual, 1)
		})
	})
}

func TestAnalyzerProcess(t *testing.T) {
	Convey("Given an incomplete signal epoch", t, func() {
		order := make([]int, 0, 1)
		mu := &sync.Mutex{}
		analyzer := &Analyzer{
			solvers: []Solver{
				&orderedSolver{index: 0, order: &order, mu: mu, source: types.SourceCategory},
			},
		}
		thesis := types.NewThesis(t.Context(), nil)
		analyzer.Process(thesis)

		Convey("It should avoid running solvers without active symbols", func() {
			So(order, ShouldBeEmpty)
		})
	})

	Convey("Given several signal notifications for one readiness epoch", t, func() {
		order := make([]int, 0, 6)
		mu := &sync.Mutex{}
		analyzer := &Analyzer{
			solvers: []Solver{
				&orderedSolver{index: 0, order: &order, mu: mu, source: types.SourceCategory},
				&orderedSolver{index: 1, order: &order, mu: mu, source: types.SourceManifold},
				&orderedSolver{index: 2, order: &order, mu: mu, source: types.SourceResonance},
				&orderedSolver{index: 3, order: &order, mu: mu, source: types.SourceCausal},
				&orderedSolver{index: 4, order: &order, mu: mu, source: types.SourceCognition},
				&orderedSolver{index: 5, order: &order, mu: mu, source: types.SourceGraph},
			},
		}
		thesis := types.NewThesis(t.Context(), nil)

		analyzer.Process(thesis)
		analyzer.Process(thesis)
		slices.Sort(order)

		Convey("It should not rerun a completed symbol for a duplicate notification", func() {
			So(order, ShouldResemble, []int{0, 1, 2, 3, 4, 5})
		})
	})

	Convey("Given a solver that cannot complete the current logic cut", t, func() {
		order := make([]int, 0, 3)
		mu := &sync.Mutex{}
		analyzer := &Analyzer{
			solvers: []Solver{
				&orderedSolver{index: 0, order: &order, mu: mu, source: types.SourceCategory},
				&orderedSolver{index: 1, order: &order, mu: mu, err: errors.New("failed cut")},
				&orderedSolver{index: 2, order: &order, mu: mu, source: types.SourceGraph},
			},
		}
		thesis := types.NewThesis(t.Context(), nil)

		analyzer.Process(thesis)
		slices.Sort(order)

		Convey("It should let independent solvers complete their available work", func() {
			So(order, ShouldResemble, []int{0, 1, 2})
		})
	})

	Convey("Given a pass in which no solver can advance readiness", t, func() {
		order := make([]int, 0, 1)
		mu := &sync.Mutex{}
		analyzer := &Analyzer{
			solvers: []Solver{
				&orderedSolver{index: 0, order: &order, mu: mu},
			},
		}
		thesis := types.NewThesis(t.Context(), nil)
		symbol := types.NewSymbol("BTC/USD", nil)
		symbol.Status = types.BUSY
		thesis.Symbols.Store("BTC/USD", symbol)

		analyzer.Process(thesis)

		Convey("It should stop after the first unchanged pass", func() {
			So(order, ShouldResemble, []int{0})
			So(symbol.Status, ShouldEqual, types.BUSY)
		})
	})

	Convey("Given incomplete signal contributions for one symbol", t, func() {
		order := make([]int, 0, 1)
		mu := &sync.Mutex{}
		analyzer := &Analyzer{
			solvers: []Solver{
				&orderedSolver{
					index: 0, order: &order, mu: mu, source: types.SourceGraph,
				},
			},
		}
		thesis := types.NewThesis(t.Context(), nil)
		correlation := &types.Measurement{
			Source: types.SourceCorrelation, Symbol: "BTC/USD",
		}
		cvd := &types.Measurement{Source: types.SourceCVD, Symbol: "BTC/USD"}
		So(thesis.AppendMeasurements(
			types.SourceCorrelation, []*types.Measurement{correlation}, true,
		), ShouldBeNil)
		So(thesis.AppendMeasurements(
			types.SourceCVD, []*types.Measurement{cvd}, true,
		), ShouldBeNil)

		analyzer.Process(thesis)
		stored, _ := thesis.Symbols.Load("BTC/USD")
		symbol := stored.(*types.Symbol)

		Convey("It should wait for the complete signal set before running logic", func() {
			So(order, ShouldBeEmpty)
			So(symbol.Status, ShouldEqual, types.READY)
			So(symbol.Measurements, ShouldResemble, []*types.Measurement{correlation, cvd})
		})
	})
}

/*
BenchmarkAnalyzerProcess isolates orchestration and epoch gating with the six
production stage positions; each solver package benchmarks its own model work.
*/
func BenchmarkAnalyzerProcess(b *testing.B) {
	order := make([]int, 0, 6)
	mu := &sync.Mutex{}
	analyzer := &Analyzer{
		solvers: []Solver{
			&orderedSolver{index: 0, order: &order, mu: mu, source: types.SourceCategory},
			&orderedSolver{index: 1, order: &order, mu: mu, source: types.SourceManifold},
			&orderedSolver{index: 2, order: &order, mu: mu, source: types.SourceResonance},
			&orderedSolver{index: 3, order: &order, mu: mu, source: types.SourceCausal},
			&orderedSolver{index: 4, order: &order, mu: mu, source: types.SourceCognition},
			&orderedSolver{index: 5, order: &order, mu: mu, source: types.SourceGraph},
		},
	}
	thesis := types.NewThesis(b.Context(), nil)
	b.ReportAllocs()

	for b.Loop() {
		value, found := thesis.Symbols.Load("BTC/USD")

		if found {
			value.(*types.Symbol).Reset()
		}

		order = order[:0]
		analyzer.Process(thesis)
	}
}
