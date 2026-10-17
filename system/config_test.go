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
			So(Cfg.Planner, ShouldNotBeNil)
			So(Cfg.UI, ShouldNotBeNil)
			So(Cfg.Workbench, ShouldNotBeNil)
			So(Cfg.Audit, ShouldNotBeNil)
		})

		Convey("The gate has no defaults: unconfigured, Validate names every missing key", func() {
			err := NewResonance().Validate()
			So(err, ShouldNotBeNil)

			for _, key := range []string{"resonance.gate", "resonance.surprise_z", "resonance.learning_rate"} {
				So(err.Error(), ShouldContainSubstring, key+" is not configured")
			}
		})

		Convey("It should construct independent config instances", func() {
			config := NewConfig()
			So(config, ShouldNotBeNil)
			So(config.Resonance, ShouldNotBeNil)
			So(config.Risk, ShouldNotBeNil)
			So(config.Planner, ShouldNotBeNil)
			So(config.UI, ShouldNotBeNil)
			So(config.Workbench, ShouldNotBeNil)
			So(config.Audit, ShouldNotBeNil)
		})
	})
}
