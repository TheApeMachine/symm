package logic

import (
	"errors"
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
	source types.SourceType
	err    error
}

func (solver *orderedSolver) Update(thesis *types.Thesis) error {
	*solver.order = append(*solver.order, solver.index)

	if solver.err != nil {
		return solver.err
	}

	if solver.source != "" {
		thesis.Stamp(solver.source)
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
	Convey("Given several signal notifications for one readiness epoch", t, func() {
		order := make([]int, 0, 6)
		analyzer := &Analyzer{
			solvers: []Solver{
				&orderedSolver{index: 0, order: &order, source: types.SourceCategories},
				&orderedSolver{index: 1, order: &order, source: types.SourceManifold},
				&orderedSolver{index: 2, order: &order, source: types.SourceResonance},
				&orderedSolver{index: 3, order: &order, source: types.SourceCausal},
				&orderedSolver{index: 4, order: &order, source: types.SourceCognition},
				&orderedSolver{index: 5, order: &order, source: types.SourceGraph},
			},
		}
		thesis := types.NewThesis(t.Context(), nil)
		stampSignalReadiness(thesis)

		analyzer.process(thesis)
		analyzer.process(thesis)

		Convey("It should freeze the completed dependency-consistent cut", func() {
			So(order, ShouldResemble, []int{0, 1, 2, 3, 4, 5})
		})
	})

	Convey("Given a solver that cannot complete the current logic cut", t, func() {
		order := make([]int, 0, 3)
		analyzer := &Analyzer{
			solvers: []Solver{
				&orderedSolver{index: 0, order: &order, source: types.SourceCategories},
				&orderedSolver{index: 1, order: &order, err: errors.New("failed cut")},
				&orderedSolver{index: 2, order: &order, source: types.SourceGraph},
			},
		}
		thesis := types.NewThesis(t.Context(), nil)
		stampSignalReadiness(thesis)

		analyzer.process(thesis)

		Convey("It should not publish downstream stages from the partial snapshot", func() {
			So(order, ShouldResemble, []int{0, 1})
			So(thesis.Readiness.Graph, ShouldBeFalse)
		})
	})
}

func stampSignalReadiness(thesis *types.Thesis) {
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
		thesis.Stamp(source)
	}
}

/*
BenchmarkAnalyzerProcess isolates orchestration and epoch gating with the six
production stage positions; each solver package benchmarks its own model work.
*/
func BenchmarkAnalyzerProcess(b *testing.B) {
	order := make([]int, 0, 6)
	analyzer := &Analyzer{
		solvers: []Solver{
			&orderedSolver{index: 0, order: &order, source: types.SourceCategories},
			&orderedSolver{index: 1, order: &order, source: types.SourceManifold},
			&orderedSolver{index: 2, order: &order, source: types.SourceResonance},
			&orderedSolver{index: 3, order: &order, source: types.SourceCausal},
			&orderedSolver{index: 4, order: &order, source: types.SourceCognition},
			&orderedSolver{index: 5, order: &order, source: types.SourceGraph},
		},
	}
	thesis := types.NewThesis(b.Context(), nil)
	b.ReportAllocs()

	for b.Loop() {
		thesis.Readiness.Reset()
		stampSignalReadiness(thesis)
		order = order[:0]
		analyzer.process(thesis)
	}
}
