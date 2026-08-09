package category

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestMapperUpdate(t *testing.T) {
	Convey("Given the independent evidence required for a coiled compression", t, func() {
		mapper := NewMapper()
		compression, compressionFound := mapper.Weights(types.MetricCompression)
		drive, driveFound := mapper.Weights(types.MetricDrive)

		Convey("Then compression and aggressive flow should support the hypothesis", func() {
			So(compressionFound, ShouldBeTrue)
			So(driveFound, ShouldBeTrue)
			So(compression[types.CategoryCoiledCompression], ShouldBeGreaterThan, 0.0)
			So(drive[types.CategoryCoiledCompression], ShouldBeGreaterThan, 0.0)
		})
	})
}
