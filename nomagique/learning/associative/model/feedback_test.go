package model

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestModelFeedback(t *testing.T) {
	Convey("Provisional rewards never settle a decision or replace completed evidence", t, func() {
		learned := New[string, string]()
		identity, err := learned.Issue("task", []uint64{1, 2}, "work", 1)
		So(err, ShouldBeNil)
		So(learned.Feedback(identity, 4), ShouldBeNil)
		So(learned.Feedback(identity, 6), ShouldBeNil)
		reading := learned.Recall("task", []uint64{1, 2}, "work")
		So(reading.Mean, ShouldEqual, 5)
		So(reading.Provisional, ShouldBeTrue)
		So(reading.Pending, ShouldEqual, 1)

		Convey("A completed negative result takes precedence over positive provisional rewards", func() {
			_, err := learned.Resolve(identity, -3)
			So(err, ShouldBeNil)
			reading = learned.Recall("task", []uint64{1, 2}, "work")
			So(reading.Mean, ShouldEqual, -3)
			So(reading.Provisional, ShouldBeFalse)
			So(reading.Pending, ShouldEqual, 0)
			So(learned.Feedback(identity, 100), ShouldNotBeNil)
		})

		Convey("Unknown tickets cannot introduce training evidence", func() {
			So(learned.Feedback(identity+1, 100), ShouldNotBeNil)
			So(learned.Recall("task", []uint64{1, 2}, "work"), ShouldResemble, reading)
		})

		Convey("Completed broader evidence outranks a provisional specific context", func() {
			So(learned.Observe("task", nil, "work", -2, 1), ShouldBeNil)
			reading = learned.Recall("task", []uint64{1, 2}, "work")
			So(reading.Mean, ShouldEqual, -2)
			So(reading.Provisional, ShouldBeFalse)
			So(reading.Depth, ShouldEqual, 0)
		})
	})
}

func BenchmarkModelFeedback(b *testing.B) {
	learned := New[string, string]()
	identity, err := learned.Issue("task", []uint64{1, 2, 3}, "work", 1)

	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()

	for b.Loop() {
		if err := learned.Feedback(identity, -1); err != nil {
			b.Fatal(err)
		}
	}
}
