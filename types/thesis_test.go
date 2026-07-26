package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNoteLifecycle(t *testing.T) {
	Convey("Given a thesis phase transition", t, func() {
		thesis := NewThesis()
		at := time.Unix(1, 0).UTC()
		thesis.NoteLifecycle("BTC/USD", LifecycleEntered, at)

		Convey("It should store the phase without a parallel journal", func() {
			phase, ok := thesis.Lifecycle.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(phase, ShouldEqual, LifecycleEntered)
		})
	})
}

func TestThesisSaveCheckpoint(t *testing.T) {
	Convey("Given a finalized immutable cut", t, func() {
		dir := t.TempDir()
		thesis := NewThesis()
		cut := &ImmutableCut{
			ID:   2,
			Tick: 4,
			At:   time.Unix(1, 0).UTC(),
		}

		Convey("Save delegates to cut checkpoint", func() {
			So(thesis.Save(dir, cut), ShouldBeNil)
			So(thesis.Save(dir, nil), ShouldNotBeNil)
		})
	})
}

func BenchmarkNoteLifecycle(b *testing.B) {
	thesis := NewThesis()
	at := time.Unix(1, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		thesis.NoteLifecycle("BTC/USD", LifecycleManaging, at)
	}
}
