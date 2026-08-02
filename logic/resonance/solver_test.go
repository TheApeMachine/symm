package resonance

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestUpdate(t *testing.T) {
	Convey("Given a resonance solver with an initialized feature schema", t, func() {
		solver := NewSolver(nil, nil)
		thesis := resonanceTestThesis(map[string]types.MetricSample{
			"drive": {Raw: 0.4},
		})

		So(solver.Update(thesis), ShouldBeNil)
		initialManifold := solver.manifold

		Convey("It should retain the learned manifold when the schema is unchanged", func() {
			thesis = resonanceTestThesis(map[string]types.MetricSample{
				"drive": {Raw: 0.7},
			})

			So(solver.Update(thesis), ShouldBeNil)
			So(solver.manifold, ShouldEqual, initialManifold)
		})

		Convey("It should retain the learned manifold when an established feature is temporarily absent", func() {
			thesis = resonanceTestThesis(map[string]types.MetricSample{
				"balance": {Raw: -0.2},
			})
			So(solver.Update(thesis), ShouldBeNil)
			expandedManifold := solver.manifold

			thesis = resonanceTestThesis(map[string]types.MetricSample{
				"drive": {Raw: 0.7},
			})

			So(solver.Update(thesis), ShouldBeNil)
			So(solver.manifold, ShouldEqual, expandedManifold)
		})

		Convey("It should rebuild the manifold before settling a changed schema", func() {
			thesis = resonanceTestThesis(map[string]types.MetricSample{
				"drive":   {Raw: 0.7},
				"balance": {Raw: -0.2},
			})

			So(solver.Update(thesis), ShouldBeNil)
			So(solver.manifold, ShouldNotEqual, initialManifold)
			So(solver.featureSchema, ShouldResemble, []string{
				"cvd:BTC/USD::balance",
				"cvd:BTC/USD::drive",
			})
		})
	})
}

func resonanceTestThesis(metrics map[string]types.MetricSample) *types.Thesis {
	thesis := types.NewThesis()
	thesis.Measurements.Store(types.SourceCVD, []*types.Measurement{{
		Source:  types.SourceCVD,
		Symbol:  "BTC/USD",
		At:      time.Unix(1, 0).UTC(),
		Metrics: metrics,
	}})

	return thesis
}
