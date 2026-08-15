package logic

import (
	"errors"
	"fmt"
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
	index   int
	order   *[]int
	mu      *sync.Mutex
	source  types.SourceType
	err     error
	symbols *[]string
}

func (solver *orderedSolver) Update(thesis *types.Thesis) error {
	solver.mu.Lock()
	*solver.order = append(*solver.order, solver.index)

	if solver.symbols != nil {
		thesis.Symbols.Range(func(key, _ any) bool {
			*solver.symbols = append(*solver.symbols, key.(string))
			return true
		})
	}

	solver.mu.Unlock()

	if solver.err != nil {
		return solver.err
	}

	return nil
}

func (solver *orderedSolver) Name() string {
	if solver.source != "" {
		return string(solver.source)
	}

	return fmt.Sprintf("solver-%d", solver.index)
}

func (solver *orderedSolver) Close() error {
	return nil
}

func TestNewAnalyzer(t *testing.T) {
	Convey("Given analyzer construction", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		analyzer := NewAnalyzer(
			t.Context(), nil, nil, tree, nil, nil, nil, thesis,
		)
		defer analyzer.Close()

		Convey("Then it should build dependency levels over the production solvers", func() {
			So(analyzer.solvers, ShouldHaveLength, 6)
			So(analyzer.solverGroups, ShouldHaveLength, 3)
			So(analyzer.solverGroups[0], ShouldHaveLength, 3)
			So(analyzer.solverGroups[1], ShouldHaveLength, 2)
			So(analyzer.solverGroups[2], ShouldHaveLength, 1)
		})
	})
}

func TestAnalyzerProcess(t *testing.T) {
	Convey("Given solvers in a single dependency group", t, func() {
		order := make([]int, 0, 2)
		mu := &sync.Mutex{}
		first := &orderedSolver{index: 0, order: &order, mu: mu}
		resonance := &orderedSolver{index: 1, order: &order, mu: mu}
		analyzer := &Analyzer{
			ctx:          t.Context(),
			resonance:    resonance,
			solverGroups: [][]Solver{{first, resonance}},
		}
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbol("BTC/USD")

		err := analyzer.Process(thesis)
		slices.Sort(order)

		Convey("It should invoke every solver in the group", func() {
			So(err, ShouldBeNil)
			So(order, ShouldResemble, []int{0, 1})
		})
	})

	Convey("Given dependency levels with independent solvers", t, func() {
		order := make([]int, 0, 6)
		mu := &sync.Mutex{}
		first := &orderedSolver{index: 0, order: &order, mu: mu}
		second := &orderedSolver{index: 1, order: &order, mu: mu}
		third := &orderedSolver{index: 2, order: &order, mu: mu}
		fourth := &orderedSolver{index: 3, order: &order, mu: mu}
		fifth := &orderedSolver{index: 4, order: &order, mu: mu}
		sixth := &orderedSolver{index: 5, order: &order, mu: mu}
		analyzer := &Analyzer{
			ctx: t.Context(),
			solverGroups: [][]Solver{
				{first, second, third},
				{fourth, fifth},
				{sixth},
			},
		}
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbol("BTC/USD")

		err := analyzer.Process(thesis)

		Convey("It should complete each dependency level before starting the next", func() {
			So(err, ShouldBeNil)
			firstLevel := slices.Clone(order[:3])
			secondLevel := slices.Clone(order[3:5])
			slices.Sort(firstLevel)
			slices.Sort(secondLevel)
			So(firstLevel, ShouldResemble, []int{0, 1, 2})
			So(secondLevel, ShouldResemble, []int{3, 4})
			So(order[5], ShouldEqual, 5)
		})
	})

	Convey("Given unrelated symbols carrying analysis state", t, func() {
		order := make([]int, 0, 1)
		processedSymbols := make([]string, 0, 2)
		mu := &sync.Mutex{}
		analyzer := &Analyzer{
			ctx: t.Context(),
			solverGroups: [][]Solver{{
				&orderedSolver{
					index:   0,
					order:   &order,
					mu:      mu,
					symbols: &processedSymbols,
				},
			}},
		}
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbol("BTC/USD")
		thesis.Symbol("ETH/USD")

		err := analyzer.Process(thesis)
		slices.Sort(processedSymbols)

		Convey("It should expose every symbol on the thesis", func() {
			So(err, ShouldBeNil)
			So(processedSymbols, ShouldResemble, []string{"BTC/USD", "ETH/USD"})
		})
	})

	Convey("Given a solver that cannot complete its dependency level", t, func() {
		order := make([]int, 0, 3)
		mu := &sync.Mutex{}
		analyzer := &Analyzer{
			ctx: t.Context(),
			solverGroups: [][]Solver{
				{
					&orderedSolver{index: 0, order: &order, mu: mu},
					&orderedSolver{index: 1, order: &order, mu: mu, err: errors.New("failed cut")},
				},
				{&orderedSolver{index: 2, order: &order, mu: mu}},
			},
		}
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbol("BTC/USD")

		err := analyzer.Process(thesis)
		slices.Sort(order)

		Convey("It should finish the current level and skip dependent levels", func() {
			So(err, ShouldNotBeNil)
			So(order, ShouldResemble, []int{0, 1})
		})
	})

}

func TestSettling(t *testing.T) {
	Convey("Given an analyzer with no relaxing solvers", t, func() {
		analyzer := &Analyzer{}

		Convey("It should report idle", func() {
			So(analyzer.Settling(), ShouldBeFalse)
			So((*Analyzer)(nil).Settling(), ShouldBeFalse)
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
		ctx: b.Context(),
		solverGroups: [][]Solver{
			{
				&orderedSolver{index: 0, order: &order, mu: mu},
				&orderedSolver{index: 1, order: &order, mu: mu},
				&orderedSolver{index: 2, order: &order, mu: mu},
			},
			{
				&orderedSolver{index: 3, order: &order, mu: mu},
				&orderedSolver{index: 4, order: &order, mu: mu},
			},
			{&orderedSolver{index: 5, order: &order, mu: mu}},
		},
	}
	thesis := types.NewThesis(b.Context(), nil)

	for symbolIndex := range 256 {
		thesis.Symbol(fmt.Sprintf("TEST-%03d/USD", symbolIndex))
	}

	thesis.Symbol("BTC/USD")

	for b.Loop() {
		value, found := thesis.Symbols.Load("BTC/USD")

		_ = found
		_ = value

		order = order[:0]
		analyzer.Process(thesis)
	}
}
