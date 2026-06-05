package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCurrentRegimeDefault(t *testing.T) {
	Convey("Given no published regime", t, func() {
		ResetRegimeForTest()

		Convey("CurrentRegime returns RegimeNone", func() {
			So(CurrentRegime(), ShouldEqual, RegimeNone)
		})
	})
}

func TestPublishRegime(t *testing.T) {
	Convey("Given a published regime", t, func() {
		PublishRegime(RegimeBullish)

		Convey("CurrentRegime returns the latest value", func() {
			So(CurrentRegime(), ShouldEqual, RegimeBullish)
		})
	})
}
