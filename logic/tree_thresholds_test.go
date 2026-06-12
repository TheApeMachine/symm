package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
	"go.yaml.in/yaml/v3"
)

func TestApplyConfigThresholdsBaselineSentinel(t *testing.T) {
	Convey("Given explicit baseline references and literal thresholds", t, func() {
		viper.Set("trading.entry.confidence_baseline", 0.62)
		viper.Set("trading.entry.surprise_baseline", 1.4)

		tree := &Tree{
			Branches: []*Branch{
				NewBranch(
					NewConditionGroup(BooleanTypeAnd, []Condition{
						*NewCondition(
							ConditionIsGreaterThanOrEqual,
							ConditionOperand{Subject: *NewSubject(
								SourcePumpDump,
								SubjectConfidence,
								nil,
								nil,
								nil,
								0,
								0,
								0,
								0,
								0,
								0,
								0,
							)},
							ConditionOperand{Subject: Subject{
								Type:                   SubjectConfidence,
								confidenceUsesBaseline: true,
							}},
						),
						*NewCondition(
							ConditionIsGreaterThanOrEqual,
							ConditionOperand{Subject: *NewSubject(
								SourceHawkes,
								SubjectConfidence,
								nil,
								nil,
								nil,
								0,
								0,
								0,
								0,
								0,
								0,
								0,
							)},
							ConditionOperand{Subject: *NewSubject(
								SourceNone,
								SubjectConfidence,
								nil,
								nil,
								nil,
								0,
								0,
								0,
								0,
								0,
								0.55,
								0,
							)},
						),
					}),
					nil,
				),
			},
		}

		thresholdConfig, err := config.LoadThresholdConfig()
		So(err, ShouldBeNil)

		applyConfigThresholds(tree, thresholdConfig)

		Convey("It should resolve baseline sentinels from config", func() {
			So(
				tree.Branches[0].ConditionGroup.Conditions[0].Right.Subject.Confidence,
				ShouldEqual,
				0.62,
			)
		})

		Convey("It should leave literal thresholds untouched", func() {
			So(
				tree.Branches[0].ConditionGroup.Conditions[1].Right.Subject.Confidence,
				ShouldEqual,
				0.55,
			)
		})
	})
}

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
