package hindsight

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReplayCursorStepTest(t *testing.T) {
	Convey("Given a replay cursor over a causal tape", t, func() {
		records := []ReplayRecord{
			{Position: CausalOrdering{Run: "r", Sequence: 1, Ordinal: 0}, Markers: nil},
			{Position: CausalOrdering{Run: "r", Sequence: 1, Ordinal: 1}, Markers: []string{"decision"}},
			{Position: CausalOrdering{Run: "r", Sequence: 2, Ordinal: 0}, Markers: nil},
		}

		cursor := NewReplayCursor("r", records)

		Convey("Step advances in causal order, independent of input order", func() {
			first, ok := cursor.Step()
			So(ok, ShouldBeTrue)
			So(first.Position.Sequence, ShouldEqual, CaptureSequence(1))
			So(first.Position.Ordinal, ShouldEqual, uint64(0))

			second, ok := cursor.Step()
			So(ok, ShouldBeTrue)
			So(second.Position.Ordinal, ShouldEqual, uint64(1))

			third, ok := cursor.Step()
			So(ok, ShouldBeTrue)
			So(third.Position.Sequence, ShouldEqual, CaptureSequence(2))

			_, ok = cursor.Step()
			So(ok, ShouldBeFalse)
		})
	})
}

func TestReplayCursorFastForwardTest(t *testing.T) {
	Convey("Given a cursor and a target capture sequence", t, func() {
		records := []ReplayRecord{
			{Position: CausalOrdering{Run: "r", Sequence: 1, Ordinal: 0}},
			{Position: CausalOrdering{Run: "r", Sequence: 2, Ordinal: 0}},
			{Position: CausalOrdering{Run: "r", Sequence: 3, Ordinal: 0}},
		}

		cursor := NewReplayCursor("r", records)

		Convey("FastForward skips causal deltas and rests at the target", func() {
			skipped := cursor.FastForward(3)

			So(len(skipped), ShouldEqual, 2)
			So(cursor.Position().Sequence, ShouldEqual, CaptureSequence(2))
		})
	})
}

func TestReplayCursorNextMarkedTest(t *testing.T) {
	Convey("Given a cursor with one marked record", t, func() {
		records := []ReplayRecord{
			{Position: CausalOrdering{Run: "r", Sequence: 1, Ordinal: 0}},
			{Position: CausalOrdering{Run: "r", Sequence: 2, Ordinal: 0}, Markers: []string{"entry"}},
			{Position: CausalOrdering{Run: "r", Sequence: 3, Ordinal: 0}},
		}

		cursor := NewReplayCursor("r", records)

		Convey("NextMarked stops at the next marked event", func() {
			record, ok := cursor.NextMarked()
			So(ok, ShouldBeTrue)
			So(record.Markers, ShouldContain, "entry")
			So(record.Position.Sequence, ShouldEqual, CaptureSequence(2))
		})
	})
}

func TestCausalOrderingLessTest(t *testing.T) {
	Convey("Given two causal positions", t, func() {
		Convey("ordering is deterministic over sequence then ordinal then version", func() {
			first := CausalOrdering{Run: "r", Sequence: 1, Ordinal: 1}
			second := CausalOrdering{Run: "r", Sequence: 1, Ordinal: 2}

			So(first.Less(second), ShouldBeTrue)
			So(second.Less(first), ShouldBeFalse)
		})
	})
}
