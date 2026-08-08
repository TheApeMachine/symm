package logic

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
orderedSolver records when Analyzer invokes a solver.
*/
type orderedSolver struct {
	index  int
	order  *[]int
	source types.SourceType
}

func (solver *orderedSolver) Update(thesis *types.Thesis) error {
	*solver.order = append(*solver.order, solver.index)

	if solver.source != "" {
		thesis.Stamp(solver.source)
	}

	return nil
}

func (solver *orderedSolver) Close() error {
	return nil
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

		Convey("It should run one dependency-consistent cut", func() {
			So(order, ShouldResemble, []int{0, 1, 2, 3, 4, 5})
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
	subscribers := &sync.Map{}
	subscription := &types.Subscription[any]{Channel: make(chan any, 1)}
	subscribers.Store("analyzer", []*types.Subscription[any]{subscription})
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
		<-subscription.Channel
	}
}
