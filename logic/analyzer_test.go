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

	if solver.source != "" {
		thesis.Stamp("BTC/USD", solver.source)
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
		thesis.Subscribe(types.SourceCorrelation, signal)
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		analyzer := NewAnalyzer(
			t.Context(), nil, nil, tree, nil, nil, nil, thesis,
		)
		defer analyzer.Close()

		thesis.Fanout(types.SourceTrader)

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
		analyzer.process(thesis)

		Convey("It should let each solver inspect symbol readiness", func() {
			So(order, ShouldResemble, []int{0})
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
		stampSignalReadiness(thesis)

		analyzer.process(thesis)
		analyzer.process(thesis)
		slices.Sort(order)

		Convey("It should run the chain for each notification without a global cut", func() {
			So(order, ShouldResemble, []int{0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5})
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
		stampSignalReadiness(thesis)

		analyzer.process(thesis)
		slices.Sort(order)

		Convey("It should let independent solvers complete their available work", func() {
			So(order, ShouldResemble, []int{0, 1, 2})
			So(thesis.Stamped("BTC/USD", types.SourceGraph), ShouldBeTrue)
		})
	})
}

func stampSignalReadiness(thesis *types.Thesis) {
	thesis.Symbols.Store("BTC/USD", &types.Symbol{})

	for _, source := range []types.SourceType{
		types.SourceCorrelation,
		types.SourceCVD,
		types.SourceDepthFlow,
		types.SourceExhaustion,
		types.SourceHawkes,
		types.SourceLeadLag,
		types.SourceLiquidity,
		types.SourcePumpDump,
		types.SourceSentiment,
		types.SourceToxicity,
	} {
		thesis.Stamp("BTC/USD", source)
	}
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

		stampSignalReadiness(thesis)
		order = order[:0]
		analyzer.process(thesis)
	}
}
