package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPerspectiveClone(t *testing.T) {
	Convey("Given a Perspective with declared class evidence", t, func() {
		original := Perspective{
			Classes: []PerspectiveClass{{
				State:    "building",
				Evidence: []string{"cvd/rate"},
			}},
		}

		clone := original.Clone()
		clone.Classes[0].Evidence[0] = "cvd/divergence"

		Convey("Clone owns an independent copy of nested evidence", func() {
			So(original.Classes[0].Evidence[0], ShouldEqual, "cvd/rate")
		})
	})
}
