package engine

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSourcePerspective(t *testing.T) {
	Convey("Given known signal sources", t, func() {
		Convey("It should map each source to a market angle", func() {
			So(SourcePerspective("depthflow"), ShouldEqual, PerspectiveMicrostructure)
			So(SourcePerspective("leadlag"), ShouldEqual, PerspectiveCrossAsset)
			So(SourcePerspective("sentiment"), ShouldEqual, PerspectiveSentiment)
		})
	})
}
