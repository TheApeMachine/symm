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

func TestObservationStruct(t *testing.T) {
	Convey("Given an observation node", t, func() {
		observation := Observation{
			ObservationType: ObservationHolding,
			Value:           0.75,
		}

		Convey("It should retain type and value", func() {
			So(observation.ObservationType, ShouldEqual, ObservationHolding)
			So(observation.Value, ShouldEqual, 0.75)
		})
	})
}
