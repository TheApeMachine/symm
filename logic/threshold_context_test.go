package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.yaml.in/yaml/v3"
)

func TestNewThresholdContext(t *testing.T) {
	Convey("Given a warmed entry bar and elevated regime volatility", t, func() {
		thresholdCtx := NewThresholdContext(0.75, 1.0, 0.5)

		Convey("It should lower exit confidence under turbulence", func() {
			So(thresholdCtx.ExitConfidenceBaseline, ShouldBeLessThan, 0.75)
			So(thresholdCtx.ExitConfidenceBaseline, ShouldBeGreaterThanOrEqualTo, exitConfidenceFloor)
		})

		Convey("It should preserve the entry bar and macro temperature", func() {
			So(thresholdCtx.EntryConfidenceBaseline, ShouldAlmostEqual, 0.75, 1e-9)
			So(thresholdCtx.RiskTemperature, ShouldAlmostEqual, 0.5, 1e-9)
		})

		Convey("A cold market keeps the supplied entry bar", func() {
			cold := NewThresholdContext(0.55, 0, 0)
			So(cold.EntryConfidenceBaseline, ShouldAlmostEqual, 0.55, 1e-9)
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
