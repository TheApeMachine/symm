package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCategoryAccumulatorPrimarySource(t *testing.T) {
	Convey("Given tied source counts", t, func() {
		accumulator := &categoryAccumulator{
			sourceCount: map[SourceType]int{
				SourceCVD:      3,
				SourceToxicity: 3,
				SourceFluid:    3,
			},
		}

		Convey("It should break ties by source name", func() {
			So(accumulator.primarySource(), ShouldEqual, SourceCVD)
		})
	})
}
