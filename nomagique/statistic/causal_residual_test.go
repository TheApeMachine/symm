package statistic_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestResidualBaselineNext(t *testing.T) {
	Convey("ResidualBaseline yields the causal baseline of a residual reading", t, func() {
		result := statistic.CausalResidualResult{Baseline: 1.5, Residual: 0.25, ZScore: 2}
		out := tests.CollectSeq[float64](
			statistic.NewResidualBaseline().Next(sequence.NewValues(result).Next(nil)),
		)
		So(out, ShouldResemble, []float64{1.5})
	})
}
