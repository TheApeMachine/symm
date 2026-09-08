package model

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestModelClone(t *testing.T) {
	Convey("A new agent inherits evidence without sharing mutable priors or pending work", t, func() {
		learned := New[string, string]()
		So(learned.Observe("task", []uint64{1, 2}, "work", -2, 1), ShouldBeNil)
		identity, err := learned.Issue("task", []uint64{1, 2}, "work", 1)
		So(err, ShouldBeNil)
		So(learned.Feedback(identity, 4), ShouldBeNil)
		cloned := learned.Clone()
		reading := cloned.Recall("task", []uint64{1, 2}, "work")
		So(reading.Mean, ShouldEqual, -2)
		So(reading.Pending, ShouldEqual, 0)
		So(cloned.Sequence, ShouldEqual, learned.Sequence)
		So(cloned.Feedback(identity, 1), ShouldNotBeNil)

		So(cloned.Observe("task", []uint64{1, 2}, "work", 6, 1), ShouldBeNil)
		So(cloned.Recall("task", []uint64{1, 2}, "work").Mean, ShouldEqual, 2)
		So(learned.Recall("task", []uint64{1, 2}, "work").Mean, ShouldEqual, -2)
		So(learned.Recall("task", []uint64{1, 2}, "work").Pending, ShouldEqual, 1)
	})
}

func BenchmarkModelClone(b *testing.B) {
	learned := New[int, int]()

	// Multiple tasks and actions share prefixes but own separate evidence.
	for key := range 64 {
		for action := range 4 {
			if err := learned.Observe(key, []uint64{1, 2, 3}, action, -1, 1); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportAllocs()

	for b.Loop() {
		learned.Clone()
	}
}
