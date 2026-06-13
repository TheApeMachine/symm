package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
	"go.yaml.in/yaml/v3"
)

func TestNewThresholdContext(t *testing.T) {
	Convey("Given exit baseline config and elevated regime volatility", t, func() {
		viper.Set("trading.entry.confidence_baseline", 0.55)
		viper.Set("trading.exit.confidence_baseline", 0.50)
		viper.Set("trading.entry.turbulence_confidence_scale", 0.30)
		viper.Set("trading.exit.confidence_floor", 0.35)
		viper.Set("trading.entry.temperature_confidence_scale", 0.40)
		viper.Set("trading.entry.confidence_ceiling", 0.90)

		thresholdConfig, err := config.LoadThresholdConfig()
		So(err, ShouldBeNil)

		thresholdCtx := NewThresholdContext(thresholdConfig, 1.0, 0.5)

		Convey("It should lower exit confidence under turbulence", func() {
			So(thresholdCtx.ExitConfidenceBaseline, ShouldAlmostEqual, 0.35, 1e-9)
		})

		Convey("It should raise the entry bar with the macro temperature", func() {
			// 0.55 + 0.40*0.5 = 0.75
			So(thresholdCtx.EntryConfidenceBaseline, ShouldAlmostEqual, 0.75, 1e-9)
		})

		Convey("A cold market leaves the entry bar at the base", func() {
			cold := NewThresholdContext(thresholdConfig, 0, 0)
			So(cold.EntryConfidenceBaseline, ShouldAlmostEqual, 0.55, 1e-9)
		})

		Convey("The entry bar is capped at the ceiling when very hot", func() {
			hot := NewThresholdContext(thresholdConfig, 0, 1.0)
			So(hot.EntryConfidenceBaseline, ShouldAlmostEqual, 0.90, 1e-9)
		})
	})
}

func TestSubjectUnmarshalYAMLExitBaseline(t *testing.T) {
	Convey("Given a subject with exit_baseline confidence", t, func() {
		subject := &Subject{}
		node := &yaml.Node{}

		So(yaml.Unmarshal([]byte(`
type: confidence
confidence: exit_baseline
`), node), ShouldBeNil)

		err := subject.UnmarshalYAML(node)

		Convey("It should mark the exit baseline sentinel", func() {
			So(err, ShouldBeNil)
			So(subject.confidenceUsesExitBaseline, ShouldBeTrue)
		})
	})
}
