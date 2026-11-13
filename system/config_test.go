package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewConfig(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewConfig is called", func() {
			config := NewConfig()

			Convey("every subsection is constructed", func() {
				So(config, ShouldNotBeNil)
				So(config.Resonance, ShouldNotBeNil)
				So(config.Risk, ShouldNotBeNil)
				So(config.Selection, ShouldNotBeNil)
				So(config.Learning, ShouldNotBeNil)
				So(config.UI, ShouldNotBeNil)
				So(config.Workbench, ShouldNotBeNil)
			})
		})

		Convey("When the package has initialized", func() {
			Convey("global Cfg matches NewConfig shape", func() {
				So(Cfg, ShouldNotBeNil)
				So(Cfg.Resonance, ShouldNotBeNil)
				So(Cfg.Risk, ShouldNotBeNil)
				So(Cfg.Learning, ShouldNotBeNil)
				So(Cfg.UI, ShouldNotBeNil)
				So(Cfg.Workbench, ShouldNotBeNil)
			})
		})
	})
}
