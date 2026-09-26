package store

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/internal/dependence"
	"math"
	"testing"
)

func TestRelationCohortAdmit(t *testing.T) {
	Convey("Peer support weights one causal cross section with no out-of-domain Fisher values", t, func() {
		cohort := relationCohort{}
		cohort.admit(dependence.Estimate{Correlation: .4, Support: 3}, 2)
		cohort.admit(dependence.Estimate{Correlation: -.2, Support: 2}, 4)
		cohort.admit(dependence.Estimate{Correlation: 1.3, Support: 100}, 999)
		var values [17]float64
		var present [17]bool
		cohort.publish(func(index int, value float64) { values[index] = value }, func(index int, value bool) { present[index] = value })
		So(values[11], ShouldAlmostEqual, .16)
		So(values[12], ShouldAlmostEqual, .32)
		So(values[13], ShouldAlmostEqual, 2.8)
		So(values[14], ShouldEqual, 2)
		So(values[15], ShouldAlmostEqual, 25.0/13)
		first, second := math.Atanh(.4), math.Atanh(-.2)
		mean := (3*first + 2*second) / 5
		So(values[16], ShouldAlmostEqual, math.Sqrt((3*first*first+2*second*second)/5-mean*mean))
		So(present[16], ShouldBeTrue)
		Convey("Perfect relation is real while its infinite Fisher coordinate stays undefined", func() {
			cohort.admit(dependence.Estimate{Correlation: 1, Support: 2}, 1)
			clear(present[:])
			cohort.publish(func(index int, value float64) { values[index] = value }, func(index int, value bool) { present[index] = value })
			So(present[11], ShouldBeTrue)
			So(present[16], ShouldBeFalse)
		})
	})
}
