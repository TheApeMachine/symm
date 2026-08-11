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

func TestSnapshot(t *testing.T) {
	Convey("Given a populated system configuration", t, func() {
		config := NewConfig()
		snapshot := config.Snapshot()

		Convey("It should return an independent value graph", func() {
			So(snapshot, ShouldNotBeNil)
			So(snapshot == config, ShouldBeFalse)
			So(snapshot.Resonance == config.Resonance, ShouldBeFalse)
			So(snapshot.Manifold == config.Manifold, ShouldBeFalse)
			So(snapshot.Risk == config.Risk, ShouldBeFalse)
			So(snapshot.Planner == config.Planner, ShouldBeFalse)
			So(snapshot.Planner, ShouldResemble, config.Planner)
		})
	})
}

func TestApplyRegulation(t *testing.T) {
	Convey("Given a regulator-owned resonance and planner update", t, func() {
		config := NewConfig()
		resonance := *config.Resonance
		planner := *config.Planner
		resonance.LearningRate /= float64(config.Resonance.Layers)
		planner.MCTSIterations /= config.Manifold.RelaxationSteps

		err := config.ApplyRegulation(resonance, planner)
		snapshot := config.Snapshot()

		Convey("It should publish both settings in one configuration generation", func() {
			So(err, ShouldBeNil)
			So(snapshot.Resonance.LearningRate, ShouldEqual, resonance.LearningRate)
			So(snapshot.Planner.MCTSIterations, ShouldEqual, planner.MCTSIterations)
		})
	})
}
