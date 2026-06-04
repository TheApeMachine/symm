package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestObservationTypeConstants(t *testing.T) {
	Convey("Given observation type constants", t, func() {
		Convey("It should define a holding/not-holding pair", func() {
			So(ObservationHolding, ShouldNotEqual, ObservationNotHolding)
			So(ObservationHasStarted, ShouldBeLessThan, ObservationHasContinued)
		})
	})
}

