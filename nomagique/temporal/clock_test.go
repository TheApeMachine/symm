package temporal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestNewClock(t *testing.T) {
	Convey("Given a positive age and span", t, func() {
		clock := NewClock(1, 4)
		progress := types.NewInput[float64]()

		Convey("It should emit age over span", func() {
			So(clock.Read(progress), ShouldBeNil)
			So(progress.Value(), ShouldEqual, 0.25)
		})
	})
}

func TestWrite(t *testing.T) {
	Convey("Given another clock", t, func() {
		clock := NewClock(0, 1)
		source := NewClock(2, 8)

		Convey("It should copy age and span", func() {
			So(clock.Write(source), ShouldBeNil)
			progress := types.NewInput[float64]()
			So(clock.Read(progress), ShouldBeNil)
			So(progress.Value(), ShouldEqual, 0.25)
		})
	})
}

func TestRead(t *testing.T) {
	Convey("Given a missing span", t, func() {
		clock := NewClock(1, 0)
		progress := types.NewInput[float64]()

		Convey("It should refuse to invent a window", func() {
			So(clock.Read(progress), ShouldNotBeNil)
		})
	})
}

func TestReset(t *testing.T) {
	Convey("Given a computed clock", t, func() {
		clock := NewClock(1, 2)
		progress := types.NewInput[float64]()
		So(clock.Read(progress), ShouldBeNil)

		Convey("It should clear progress and keep the span", func() {
			So(clock.Reset(), ShouldBeNil)
			So(clock.Number(), ShouldEqual, 0)
			So(clock.Read(progress), ShouldBeNil)
			So(progress.Value(), ShouldEqual, 0.5)
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given a clock", t, func() {
		clock := NewClock(1, 2)

		Convey("It should release staged times", func() {
			So(clock.Close(), ShouldBeNil)
			progress := types.NewInput[float64]()
			So(clock.Read(progress), ShouldNotBeNil)
		})
	})
}
