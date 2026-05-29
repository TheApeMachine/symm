package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSourceContributions(t *testing.T) {
	Convey("Given repeated source measurements", t, func() {
		contributions := sourceContributions([]engine.Measurement{
			{Source: "hawkes", Confidence: 0.3},
			{Source: "hawkes", Confidence: 0.7},
			{Source: "cvd", Confidence: 0.5},
		})

		Convey("It should keep the strongest confidence per source", func() {
			So(contributions["hawkes"], ShouldEqual, 0.7)
			So(contributions["cvd"], ShouldEqual, 0.5)
		})
	})
}

func TestDominantSource(t *testing.T) {
	Convey("Given fused measurements", t, func() {
		source := dominantSource([]engine.Measurement{
			{Source: "causal", Confidence: 0.4},
			{Source: "cvd", Confidence: 0.8},
		})

		Convey("It should return the strongest source", func() {
			So(source, ShouldEqual, "cvd")
		})
	})
}

func TestRunwayForPerspective(t *testing.T) {
	Convey("Given an explicit measurement timeframe", t, func() {
		runway := runwayForPerspective(engine.Perspective{
			Measurements: []engine.Measurement{{
				Timeframe: engine.Timeframe{Start: 10, End: 20},
			}},
		})

		Convey("It should use the measurement runway", func() {
			So(runway, ShouldEqual, 10*time.Second)
		})
	})

	Convey("Given a flow perspective without timeframe", t, func() {
		runway := runwayForPerspective(engine.Perspective{
			Measurements: []engine.Measurement{{Type: engine.Flow}},
		})

		Convey("It should use the configured flow hold", func() {
			So(runway, ShouldEqual, config.System.FlowHoldBeforeExit)
		})
	})
}
