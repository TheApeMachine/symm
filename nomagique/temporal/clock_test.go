package temporal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestNewClock(t *testing.T) {
	Convey("Given an age and span", t, func() {
		age := types.NewInput(1.0)
		span := types.NewInput(2.0)

		Convey("When NewClock is called", func() {
			clock := NewClock(age, span)

			Convey("Then the returned Clock should have the correct age and span", func() {
				So(clock.age, ShouldEqual, age)
				So(clock.span, ShouldEqual, span)
			})
		})
	})
}
