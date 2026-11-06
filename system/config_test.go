package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewConfig(t *testing.T) {
	Convey("Given system package initialization", t, func() {
		Convey("It should initialize global Cfg", func() {
			So(Cfg, ShouldNotBeNil)
			So(Cfg.Resonance, ShouldNotBeNil)
			So(Cfg.Risk, ShouldNotBeNil)
			So(Cfg.Learning, ShouldNotBeNil)
			So(Cfg.UI, ShouldNotBeNil)
			So(Cfg.Workbench, ShouldNotBeNil)
		})

		Convey("It should construct independent config instances", func() {
			config := NewConfig()
			So(config, ShouldNotBeNil)
			So(config.Resonance, ShouldNotBeNil)
			So(config.Risk, ShouldNotBeNil)
			So(config.Learning, ShouldNotBeNil)
			So(config.UI, ShouldNotBeNil)
			So(config.Workbench, ShouldNotBeNil)
		})
	})
}
