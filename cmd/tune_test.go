package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTunePerspectivesPath(t *testing.T) {
	Convey("Given default tune output", t, func() {
		Convey("It should return the perspectives yaml path", func() {
			So(tunePerspectivesPath(), ShouldEqual, defaultPerspectivesOutputPath)
		})
	})
}

func TestTuneCommand(t *testing.T) {
	Convey("Given the tune command", t, func() {
		Convey("It should be registered on root", func() {
			So(tuneCmd.Use, ShouldEqual, "tune")
			So(tuneCmd.Short, ShouldNotBeBlank)
			So(tuneCmd.Flags().Lookup(tuneMaxMeasurementsFlag), ShouldNotBeNil)
		})
	})
}

func TestTuneMaxMeasurements(t *testing.T) {
	Convey("Given the default tune measurement limit", t, func() {
		maxMeasurements, err := tuneMaxMeasurements(tuneCmd)

		Convey("It should leave the capture unbounded", func() {
			So(err, ShouldBeNil)
			So(maxMeasurements, ShouldEqual, 0)
		})
	})

	Convey("Given a negative tune measurement limit", t, func() {
		flag := tuneCmd.Flags().Lookup(tuneMaxMeasurementsFlag)
		previous := flag.Value.String()
		defer tuneCmd.Flags().Set(tuneMaxMeasurementsFlag, previous)

		setErr := tuneCmd.Flags().Set(tuneMaxMeasurementsFlag, "-1")
		_, err := tuneMaxMeasurements(tuneCmd)

		Convey("It should reject the flag value", func() {
			So(setErr, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}
