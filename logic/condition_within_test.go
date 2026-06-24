package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConditionIsWithin(t *testing.T) {
	Convey("Given magnitude bounds", t, func() {
		left := ConditionOperand{Type: SubjectModeShare, ModeShare: 0.4}
		right := ConditionOperand{Type: SubjectModeShare, ModeShare: 0.5}

		Convey("It should accept values inside the bound", func() {
			ok, err := ConditionIsWithin.Evaluate(nil, nil, left, right)

			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		outside := ConditionOperand{Type: SubjectModeShare, ModeShare: 0.5}
		wide := ConditionOperand{Type: SubjectModeShare, ModeShare: 0.6}

		Convey("It should reject values outside the bound", func() {
			ok, err := ConditionIsNotWithin.Evaluate(nil, nil, wide, outside)

			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})
	})
}
