package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecisionGradeFor(t *testing.T) {
	Convey("Given source classes and touch readiness", t, func() {
		Convey("Flow sources stay diagnostic without touch", func() {
			So(DecisionGradeFor(SourceCVD, false), ShouldEqual, DecisionGradeDiagnostic)
		})

		Convey("Flow sources become executable with touch", func() {
			So(DecisionGradeFor(SourceHawkes, true), ShouldEqual, DecisionGradeExecutable)
		})

		Convey("Composite sources require touch to become executable", func() {
			So(DecisionGradeFor(SourcePumpDump, false), ShouldEqual, DecisionGradeDiagnostic)
			So(DecisionGradeFor(SourcePumpDump, true), ShouldEqual, DecisionGradeExecutable)
		})
	})
}
