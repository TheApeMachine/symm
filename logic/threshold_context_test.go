package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

func TestNewThresholdContext(t *testing.T) {
	Convey("Given exit baseline config and elevated regime volatility", t, func() {
		viper.Set("trading.entry.confidence_baseline", 0.55)
		viper.Set("trading.exit.confidence_baseline", 0.50)
		viper.Set("trading.entry.turbulence_confidence_scale", 0.30)
		viper.Set("trading.exit.confidence_floor", 0.35)

		thresholdCtx := NewThresholdContext(1.0)

		Convey("It should lower exit confidence under turbulence", func() {
			So(thresholdCtx.ExitConfidenceBaseline, ShouldAlmostEqual, 0.35, 1e-9)
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
