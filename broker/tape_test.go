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

		recent, prior := tape.Uptake("Q/USD", now)
		So(recent, ShouldEqual, 200)
		So(prior, ShouldEqual, 40)
	})

	Convey("minute buckets split at the window boundary", t, func() {
		tape := NewTape()
		now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
		window := system.Cfg.Risk.TapeWindow

		// Inside recent: one minute after the prior/recent split.
		tape.Note("Q/USD", 10, now.Add(-window+time.Minute))
		// Inside prior: one minute before that split.
		tape.Note("Q/USD", 7, now.Add(-window-time.Minute))
		tape.Note("Q/USD", 3, now.Add(-2*window+time.Minute))
		tape.Note("Q/USD", 99, now.Add(-2*window-time.Minute))

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

	Convey("stale minute slots are ignored once the ring wraps", t, func() {
		tape := NewTape()
		now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

		tape.Note("Q/USD", 5, now.Add(-3*system.Cfg.Risk.TapeWindow))
		tape.Note("Q/USD", 8, now.Add(-time.Minute))

		recent, prior := tape.Uptake("Q/USD", now)
		So(recent, ShouldEqual, 8)
		So(prior, ShouldEqual, 0)
	})
}
