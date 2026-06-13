package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.yaml.in/yaml/v3"
)

func TestSubjectUnmarshalYAMLBaseline(t *testing.T) {
	Convey("Given a subject with baseline confidence", t, func() {
		subject := &Subject{}
		node := &yaml.Node{}

		So(yaml.Unmarshal([]byte(`
type: confidence
confidence: baseline
`), node), ShouldBeNil)

		err := subject.UnmarshalYAML(node)

		Convey("It should mark the baseline sentinel without a numeric value", func() {
			So(err, ShouldBeNil)
			So(subject.confidenceUsesBaseline, ShouldBeTrue)
			So(subject.Confidence, ShouldEqual, 0)
		})
	})
}
