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

func TestSnapshot(t *testing.T) {
	Convey("Given a populated system configuration", t, func() {
		config := NewConfig()
		snapshot := config.Snapshot()

		Convey("It should return an independent value graph", func() {
			So(snapshot, ShouldNotBeNil)
			So(snapshot == config, ShouldBeFalse)
			So(snapshot.Resonance == config.Resonance, ShouldBeFalse)
			So(snapshot.Risk == config.Risk, ShouldBeFalse)
			So(snapshot.Regulator == config.Regulator, ShouldBeFalse)
			So(snapshot.Planner == config.Planner, ShouldBeFalse)
			So(snapshot.Planner, ShouldResemble, config.Planner)
		})
	})
}

func TestApplyRegulation(t *testing.T) {
	Convey("Given a regulator-owned planner update", t, func() {
		config := NewConfig()
		planner := *config.Planner
		planner.MCTSIterations = 1

		err := config.ApplyRegulation(planner)
		snapshot := config.Snapshot()

		Convey("It should publish the settings in one configuration generation", func() {
			So(err, ShouldBeNil)
			So(snapshot.Planner.MCTSIterations, ShouldEqual, planner.MCTSIterations)
		})
	})
}
