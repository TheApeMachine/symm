package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewEntryReturnRequirement(t *testing.T) {
	Convey("Given entry friction and a stop fraction", t, func() {
		requirement := newEntryReturnRequirement(0.002, 0.003)

		Convey("It should scale returns by the stricter entry gate", func() {
			So(requirement.requiredEdgeReturn, ShouldAlmostEqual, 0.004)
			So(requirement.requiredRReturn, ShouldAlmostEqual, 0.006)
			So(requirement.requiredReturn, ShouldAlmostEqual, 0.006)
			So(requirement.edge(0.012), ShouldAlmostEqual, 0.01)
			So(requirement.multipleOrZero(0.012), ShouldAlmostEqual, 2)
			So(requirement.significant(0.006), ShouldBeTrue)
			So(requirement.significant(0.0059), ShouldBeFalse)
		})
	})
}

func BenchmarkNewEntryReturnRequirement(b *testing.B) {
	for b.Loop() {
		requirement := newEntryReturnRequirement(0.002, 0.003)
		_ = requirement.multipleOrZero(0.012)
	}
}
