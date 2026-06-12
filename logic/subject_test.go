package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSubjectEvaluate(t *testing.T) {
	Convey("Given a Hawkes frenzy measurement", t, func() {
		measurement := NewMeasurement(
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
		)

		Convey("It should match a Hawkes frenzy category subject", func() {
			subject := NewSubject(
				SourceHawkes,
				SubjectCategory,
				NewCategory(CategoryFrenzy),
				nil,
				nil,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			)

			matched, err := subject.Evaluate(measurement, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeTrue)
		})

		Convey("It should not match a different category subject", func() {
			subject := NewSubject(
				SourceHawkes,
				SubjectCategory,
				NewCategory(CategorySaturation),
				nil,
				nil,
				0,
				0,
				0,
				0,
				0,
				0,
				0,
			)

			matched, err := subject.Evaluate(measurement, nil)

			So(err, ShouldBeNil)
			So(matched, ShouldBeFalse)
		})
	})
}

func TestSubjectValueFrom(t *testing.T) {
	Convey("Given a measurement with surprise 2.5", t, func() {
		measurement := NewMeasurement(
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
			0,
			2.5,
		)

		subject := NewSubject(
			SourceHawkes,
			SubjectSurprise,
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
		)

		Convey("It should read surprise from the measurement", func() {
			value, ok := subject.valueFrom(measurement)

			So(ok, ShouldBeTrue)
			So(value, ShouldEqual, 2.5)
		})
	})
}
