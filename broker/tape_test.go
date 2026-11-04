package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestTapeUptake(t *testing.T) {
	Convey("quote dollars in the last window versus the window before", t, func() {
		tape := NewTape()
		now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
		window := system.Cfg.Risk.TapeWindow

		tape.Note("Q/USD", 40, now.Add(-window-time.Minute))
		tape.Note("Q/USD", 200, now.Add(-time.Minute))
		tape.Note("Q/USD", 5, now.Add(time.Second))

		recent, prior := tape.Uptake("Q/USD", now)
		So(recent, ShouldEqual, 200)
		So(prior, ShouldEqual, 40)
	})

	Convey("a print on the window edge counts in the recent window only", t, func() {
		tape := NewTape()
		now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
		window := system.Cfg.Risk.TapeWindow

		tape.Note("Q/USD", 10, now.Add(-window))
		tape.Note("Q/USD", 7, now.Add(-window-time.Nanosecond))
		tape.Note("Q/USD", 3, now.Add(-2*window))
		tape.Note("Q/USD", 99, now.Add(-2*window-time.Nanosecond))

		recent, prior := tape.Uptake("Q/USD", now)
		So(recent, ShouldEqual, 10)
		So(prior, ShouldEqual, 10)
	})

	Convey("an empty prior window is zero, and dust notes are ignored", t, func() {
		tape := NewTape()
		now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

		tape.Note("Q/USD", 0, now)
		tape.Note("", 80, now)
		tape.Note("Q/USD", 80, time.Time{})

		recent, prior := tape.Uptake("Q/USD", now)
		So(recent, ShouldEqual, 0)
		So(prior, ShouldEqual, 0)

		quiet, before := tape.Uptake("MISSING/USD", now)
		So(quiet, ShouldEqual, 0)
		So(before, ShouldEqual, 0)
	})

	Convey("prints older than two windows are dropped on the next note", t, func() {
		tape := NewTape()
		now := time.Now()

		tape.Note("Q/USD", 5, now.Add(-3*system.Cfg.Risk.TapeWindow))
		tape.Note("Q/USD", 8, now)

		So(len(tape.prints["Q/USD"]), ShouldEqual, 1)

		recent, prior := tape.Uptake("Q/USD", now)
		So(recent, ShouldEqual, 8)
		So(prior, ShouldEqual, 0)
	})
}
