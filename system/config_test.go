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
			So(Cfg.Regulator, ShouldNotBeNil)
			So(Cfg.Planner, ShouldNotBeNil)
			So(Cfg.Resonance.LearningRate, ShouldBeGreaterThan, 0)
		})

		Convey("It should construct independent config instances", func() {
			config := NewConfig()
			So(config, ShouldNotBeNil)
			So(config.Resonance, ShouldNotBeNil)
			So(config.Risk, ShouldNotBeNil)
			So(config.Regulator, ShouldNotBeNil)
			So(config.Planner, ShouldNotBeNil)
		})
	})
}

func TestPlannerPolicy(t *testing.T) {
	Convey("Given a populated system configuration", t, func() {
		config := NewConfig()
		policy, err := config.PlannerPolicy()

		Convey("It should return the planner policy by value", func() {
			So(err, ShouldBeNil)
			So(policy, ShouldResemble, *config.Planner)
		})
	})
}

func TestCognitionSwitchConfidence(t *testing.T) {
	Convey("Given a configured cognition switch boundary", t, func() {
		config := NewConfig()
		confidence, err := config.CognitionSwitchConfidence()

		Convey("It should read the scalar without allocating a configuration graph", func() {
			So(err, ShouldBeNil)
			So(confidence, ShouldEqual, config.Planner.CognitionSwitchConfidence)
		})
	})
}

func TestOptimizationConfidence(t *testing.T) {
	Convey("Given a configured passage confidence", t, func() {
		config := NewConfig()
		confidence, err := config.OptimizationConfidence()

		Convey("It should return the regulator policy directly", func() {
			So(err, ShouldBeNil)
			So(confidence, ShouldEqual, config.Regulator.OptimizationConfidence)
		})
	})
}
