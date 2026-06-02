package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestScoredClassCode(t *testing.T) {
	Convey("Given a scored pipeline", t, func() {
		scored := NewScored(mustClassifier(), adaptive.NewProduct())

		_, err := scored.Push(100, 101)

		Convey("It should expose the latest class code from the label tap", func() {
			So(err, ShouldBeNil)
			So(scored.ClassCode(), ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}
