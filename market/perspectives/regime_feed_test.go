package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPublishRegime(t *testing.T) {
	Convey("Given a published regime", t, func() {
		PublishRegime(RegimeBullish)

		Convey("CurrentRegime returns the latest value", func() {
			So(CurrentRegime(), ShouldEqual, RegimeBullish)
		})
	})
}
