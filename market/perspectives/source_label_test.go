package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSourceDisplayLabel(t *testing.T) {
	Convey("Given known and unknown source names", t, func() {
		Convey("It should return configured short labels", func() {
			So(SourceDisplayLabel("depthflow"), ShouldEqual, "Depth")
			So(SourceDisplayLabel("liquidity"), ShouldEqual, "Liquidity")
		})

		Convey("It should title-case unknown sources", func() {
			So(SourceDisplayLabel("experimental"), ShouldEqual, "Experimental")
		})
	})
}

func BenchmarkSourceDisplayLabel(b *testing.B) {
	for b.Loop() {
		_ = SourceDisplayLabel("hawkes")
	}
}
