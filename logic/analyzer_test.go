package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/types"
)

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
			So(analyzer.solverGroups, ShouldHaveLength, 3)
			So(analyzer.solverGroups[0], ShouldHaveLength, 2)
			So(analyzer.solverGroups[1], ShouldHaveLength, 2)
			So(analyzer.solverGroups[2], ShouldHaveLength, 1)
		})
	})
}

func TestAnalyzerProcess(t *testing.T) {
	Convey("Given a self-running analyzer stage", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		tree, err := dmt.NewTree("")
		So(err, ShouldBeNil)
		analyzer := NewAnalyzer(
			t.Context(), nil, nil, tree, nil, nil, nil, thesis,
		)
		defer analyzer.Close()

		Convey("Process is the no-op coordinator because each solver self-runs", func() {
			err := analyzer.Process(thesis)
			So(err, ShouldBeNil)
		})
	})
}

/*
BenchmarkAnalyzerProcess measures the coordinator overhead now that the stage is
self-running; each solver package benchmarks its own consumer loop.
*/
func BenchmarkAnalyzerProcess(b *testing.B) {
	thesis := types.NewThesis(b.Context(), nil)
	tree, _ := dmt.NewTree("")
	analyzer := NewAnalyzer(b.Context(), nil, nil, tree, nil, nil, nil, thesis)
	defer analyzer.Close()

	for b.Loop() {
		_ = analyzer.Process(thesis)
	}
}
