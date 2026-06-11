package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.yaml.in/yaml/v3"
)

func TestCategoryUnmarshalYAML(t *testing.T) {
	Convey("Given category yaml", t, func() {
		Convey("It should reject confidence on category", func() {
			category := &Category{}

			err := yaml.Unmarshal([]byte(`type: frenzy
confidence: 0.50`), category)

			So(err, ShouldNotBeNil)
		})

		Convey("It should reject surprise on category", func() {
			category := &Category{}

			err := yaml.Unmarshal([]byte(`type: frenzy
surprise: 1.0`), category)

			So(err, ShouldNotBeNil)
		})

		Convey("It should accept type only", func() {
			category := &Category{}

			err := yaml.Unmarshal([]byte(`type: frenzy`), category)

			So(err, ShouldBeNil)
			So(category.Type, ShouldEqual, CategoryFrenzy)
		})
	})
}

func TestBranchUnmarshalYAML(t *testing.T) {
	Convey("Given branch yaml", t, func() {
		Convey("It should reject branches and action on the same node", func() {
			branch := &Branch{}

			err := yaml.Unmarshal([]byte(`branches:
  - condition_group:
      boolean: and
      conditions: []
action:
  type: market
  side: buy
  fraction: 1.0`), branch)

			So(err, ShouldNotBeNil)
		})
	})
}

func TestSubjectUnmarshalYAML(t *testing.T) {
	Convey("Given subject yaml without type", t, func() {
		Convey("It should infer category from nested category block", func() {
			subject := &Subject{}

			err := yaml.Unmarshal([]byte(`category:
  type: frenzy`), subject)

			So(err, ShouldBeNil)
			So(subject.Type, ShouldEqual, SubjectCategory)
			So(subject.Category, ShouldNotBeNil)
			So(subject.Category.Type, ShouldEqual, CategoryFrenzy)
		})

		Convey("It should infer confidence from confidence field", func() {
			subject := &Subject{}

			err := yaml.Unmarshal([]byte(`confidence: 0.55`), subject)

			So(err, ShouldBeNil)
			So(subject.Type, ShouldEqual, SubjectConfidence)
			So(subject.Confidence, ShouldEqual, 0.55)
		})
	})

	Convey("Given an is_equal condition with inferred right subject", t, func() {
		condition := &Condition{}

		err := yaml.Unmarshal([]byte(`type: is_equal
left:
  subject:
    source: hawkes
    type: category
    category:
      type: frenzy
right:
  subject:
    category:
      type: frenzy`), condition)

		So(err, ShouldBeNil)
		So(condition.Right.Subject.Type, ShouldEqual, SubjectCategory)

		measurements := []Measurement{
			*NewMeasurement(
				SourceHawkes,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryFrenzy,
				RegimeTypeNone,
				PositionTypeNone,
				0.8,
				2.5,
			),
		}

		matched, err := condition.Evaluate(measurements, nil)

		So(err, ShouldBeNil)
		So(matched, ShouldBeTrue)
	})
}
