package config

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDefaultPerspectivePath(t *testing.T) {
	Convey("DefaultPerspectivePath", t, func() {
		So(DefaultPerspectivePath(), ShouldEqual, "runs/perspectives.yaml")
	})
}

func TestDefaultPerspectiveInstallPath(t *testing.T) {
	Convey("DefaultPerspectiveInstallPath", t, func() {
		So(DefaultPerspectiveInstallPath(), ShouldEqual, "config/perspectives.yaml")
	})
}

func TestPerspectiveLoadPathFor(t *testing.T) {
	Convey("Given an empty override", t, func() {
		Convey("It should resolve to the install path", func() {
			So(PerspectiveLoadPathFor(""), ShouldEqual, DefaultPerspectiveInstallPath())
		})
	})

	Convey("Given an explicit override path", t, func() {
		Convey("It should return the configured path unchanged", func() {
			So(PerspectiveLoadPathFor("runs/custom.yaml"), ShouldEqual, "runs/custom.yaml")
		})
	})
}
