package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
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

	Convey("Given measurement-backed confidence operands", t, func() {
		measurements := []*datura.Artifact{
			testMeasurementArtifact(SourceFluid, "BTC/EUR", CategoryLaminar, 0.4, 1.0),
			testMeasurementArtifact(SourceHawkes, "BTC/EUR", CategoryFrenzy, 0.8, 1.0),
		}

		Convey("It should accept confidence inside the bound", func() {
			left := ConditionOperand{Type: SubjectConfidence, Source: SourceFluid}
			right := ConditionOperand{Type: SubjectModeShare, ModeShare: 0.5}

			ok, err := ConditionIsWithin.Evaluate(measurements, nil, left, right)

			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})

		Convey("It should reject confidence outside the bound", func() {
			left := ConditionOperand{Type: SubjectConfidence, Source: SourceHawkes}
			right := ConditionOperand{Type: SubjectModeShare, ModeShare: 0.5}

			ok, err := ConditionIsNotWithin.Evaluate(measurements, nil, left, right)

			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)
		})
	})
}
