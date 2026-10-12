package data

import (
	"math"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestMetricFinalize(t *testing.T) {
	Convey("Given a metric whose stream has seen the minimum rank prior", t, func() {
		metric := NewMetric("depth_zscore:bid", 1, UnitZScore, TimescaleRollingWindow)
		state := &standardizer{}
		priors := make([]float64, 0, int(core.MinimumRankPrior))

		for index := range int(core.MinimumRankPrior) {
			value := float64(index%5) - 2
			priors = append(priors, value)
			state.observe(value)
		}

		Convey("a finite Raw is ranked against the priors, then folded in", func() {
			So(metric.finalize(state), ShouldBeNil)
			So(metric.support, ShouldEqual, core.MinimumRankPrior)
			So(metric.Standardizable(), ShouldBeTrue)
			So(metric.Standardized, ShouldAlmostEqual, midRankZ(priors, 1), 1e-12)
			So(metric.Normalized, ShouldAlmostEqual, math.Tanh(metric.Standardized), 1e-12)
			So(state.ranks.Count(), ShouldEqual, core.MinimumRankPrior+1)
		})

		Convey("an undefined Raw fails before it can poison the stream", func() {
			metric.Raw = math.NaN()

			err := metric.finalize(state)
			So(err, ShouldNotBeNil)
			So(strings.Contains(err.Error(), "raw is required"), ShouldBeTrue)
			So(state.ranks.Count(), ShouldEqual, core.MinimumRankPrior)
		})

		Convey("an infinite Raw fails the same way", func() {
			metric.Raw = math.Inf(1)

			So(metric.finalize(state), ShouldNotBeNil)
			So(state.ranks.Count(), ShouldEqual, core.MinimumRankPrior)
		})
	})

	Convey("Given a metric whose stream has a single prior observation", t, func() {
		metric := NewMetric("depth_zscore:bid", 5, UnitZScore, TimescaleRollingWindow)
		state := &standardizer{}
		state.observe(1)

		Convey("its z-score is undefined rather than zero evidence", func() {
			So(metric.finalize(state), ShouldBeNil)
			So(metric.Standardizable(), ShouldBeFalse)
			So(metric.Standardized, ShouldEqual, 0)
			So(metric.Normalized, ShouldEqual, 0)
		})
	})
}
