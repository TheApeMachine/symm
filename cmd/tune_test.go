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
		})
	})
}
