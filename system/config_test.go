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
			So(Cfg.Manifold, ShouldNotBeNil)
			So(Cfg.Risk, ShouldNotBeNil)
			So(Cfg.Planner, ShouldNotBeNil)
			So(Cfg.Resonance.LearningRate, ShouldBeGreaterThan, 0)
			So(Cfg.Manifold.RelaxationSteps, ShouldBeGreaterThan, 0)
		})

		Convey("It should construct independent config instances", func() {
			config := NewConfig()
			So(config, ShouldNotBeNil)
			So(config.Resonance, ShouldNotBeNil)
			So(config.Manifold, ShouldNotBeNil)
			So(config.Risk, ShouldNotBeNil)
			So(config.Planner, ShouldNotBeNil)
		})
	})
}
