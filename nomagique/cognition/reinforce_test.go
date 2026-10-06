package cognition

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
)

func TestReinforceNext(t *testing.T) {
	graded := func(feedback float64) float64 {
		reading, err := drive(NewReinforce(), nil, map[string]float64{
			"probability": 0.5,
			"count":       1,
			"feedback":    feedback,
			"graded":      core.Unit,
		})
		So(err, ShouldBeNil)
		updated, readErr := number(reading, "probability")
		So(readErr, ShouldBeNil)
		return updated
	}

	Convey("Signed feedback updates one association with its magnitude", t, func() {
		So(graded(1), ShouldEqual, 0.75)
		So(graded(-1), ShouldEqual, 0.25)
		So(graded(-3), ShouldEqual, 0.125)
		So(graded(0), ShouldEqual, 0.5)
	})

	Convey("An ungraded observation strengthens by the remaining unit", t, func() {
		reading, err := drive(NewReinforce(), nil, map[string]float64{
			"probability": 0.5,
			"count":       1,
			"feedback":    0,
			"graded":      0,
		})

		So(err, ShouldBeNil)
		updated, readErr := number(reading, "probability")
		So(readErr, ShouldBeNil)
		So(updated, ShouldEqual, 0.75)
	})
}
