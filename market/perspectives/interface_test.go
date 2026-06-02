package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type stubPerspective struct {
	walkCount int
}

func (perspective *stubPerspective) Walk(measurements []Measurement) Perspective {
	perspective.walkCount++

	return perspective
}

func TestPerspectiveInterface(t *testing.T) {
	Convey("Given a perspective implementation", t, func() {
		perspective := &stubPerspective{}

		Convey("It should accept measurements on Walk", func() {
			var iface Perspective = perspective
			result := iface.Walk([]Measurement{{Symbol: "BTC/EUR"}})

			So(result, ShouldEqual, perspective)
			So(perspective.walkCount, ShouldEqual, 1)
		})
	})
}
