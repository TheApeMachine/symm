package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTransition(t *testing.T) {
	Convey("Given a pending lot", t, func() {
		Convey("It accepts open", func() {
			next, err := Transition(PENDING, OPEN)
			So(err, ShouldBeNil)
			So(next, ShouldEqual, OPEN)
		})

		Convey("It rejects illegal edges", func() {
			_, err := Transition(CLOSED, PENDING)
			So(err, ShouldNotBeNil)
		})

		Convey("It normalizes cancelled to canceled", func() {
			next, err := Transition(PENDING, Status("cancelled"))
			So(err, ShouldBeNil)
			So(next, ShouldEqual, CANCELED)
		})

		Convey("It accepts adopted lot open from initializing", func() {
			next, err := Transition(INITIALIZING, OPEN)
			So(err, ShouldBeNil)
			So(next, ShouldEqual, OPEN)
		})

		Convey("It accepts wallet lot reopen from closed", func() {
			next, err := Transition(CLOSED, OPEN)
			So(err, ShouldBeNil)
			So(next, ShouldEqual, OPEN)
		})
	})
}

func TestStatusFromMarket(t *testing.T) {
	Convey("Given a known exec_type", t, func() {
		status, err := StatusFromMarket("filled")
		So(err, ShouldBeNil)
		So(status, ShouldEqual, FILLED)
	})

	Convey("Given an unknown exec_type", t, func() {
		_, err := StatusFromMarket("mystery")
		So(err, ShouldNotBeNil)
	})
}

func BenchmarkTransition(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_, _ = Transition(PENDING, OPEN)
	}
}
