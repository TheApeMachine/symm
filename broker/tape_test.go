package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestNewTape(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewTape is called", func() {
			tape := NewTape()

			Convey("It returns an empty tape", func() {
				So(tape, ShouldNotBeNil)
				recent, prior := tape.Uptake("BTC/USD", time.Unix(1_700_000_000, 0).UTC())
				So(recent, ShouldEqual, 0.0)
				So(prior, ShouldEqual, 0.0)
			})
		})
	})
}

func TestNote(t *testing.T) {
	Convey("Setup", t, func() {
		tape := NewTape()
		at := time.Unix(1_700_000_000, 0).UTC()

		Convey("When Note is called with a positive quote", func() {
			tape.Note("BTC/USD", 250.0, at)

			Convey("Uptake recent includes that notional in the current minute", func() {
				recent, prior := tape.Uptake("BTC/USD", at)
				So(recent, ShouldEqual, 250.0)
				So(prior, ShouldEqual, 0.0)
			})
		})

		Convey("When Note is called twice in the same minute", func() {
			tape.Note("BTC/USD", 100.0, at)
			tape.Note("BTC/USD", 50.0, at.Add(30*time.Second))

			Convey("Quotes accumulate in the minute bucket", func() {
				recent, _ := tape.Uptake("BTC/USD", at)
				So(recent, ShouldEqual, 150.0)
			})
		})

		Convey("When Note is called with invalid inputs", func() {
			tape.Note("", 100.0, at)
			tape.Note("ETH/USD", 0, at)
			tape.Note("ETH/USD", -1, at)
			tape.Note("ETH/USD", 100.0, time.Time{})

			Convey("Uptake stays empty", func() {
				recent, prior := tape.Uptake("ETH/USD", at)
				So(recent, ShouldEqual, 0.0)
				So(prior, ShouldEqual, 0.0)
			})
		})
	})
}

func TestUptake(t *testing.T) {
	Convey("Setup", t, func() {
		tape := NewTape()
		window := system.Cfg.Risk.TapeWindow
		So(window, ShouldEqual, 10*time.Minute)

		now := time.Unix(1_700_000_600, 0).UTC() // minute-aligned

		Convey("When recent and prior windows both have prints", func() {
			// recent window: last 10 minutes including now
			tape.Note("BTC/USD", 40.0, now)
			tape.Note("BTC/USD", 60.0, now.Add(-5*time.Minute))
			// prior window: the 10 minutes before that
			tape.Note("BTC/USD", 25.0, now.Add(-10*time.Minute))
			tape.Note("BTC/USD", 75.0, now.Add(-15*time.Minute))

			Convey("Recent and prior sum their windows exactly", func() {
				recent, prior := tape.Uptake("BTC/USD", now)
				So(recent, ShouldEqual, 100.0)
				So(prior, ShouldEqual, 100.0)
			})
		})

		Convey("When the symbol was never noted", func() {
			recent, prior := tape.Uptake("MISSING/USD", now)

			Convey("Both windows are zero", func() {
				So(recent, ShouldEqual, 0.0)
				So(prior, ShouldEqual, 0.0)
			})
		})
	})
}

func TestLane(t *testing.T) {
	Convey("Setup", t, func() {
		tape := NewTape()
		at := time.Unix(10_000, 0)
		buckets := 2 * int(system.Cfg.Risk.TapeWindow/time.Minute)

		Convey("When the first print allocates storage", func() {
			tape.Note(testSymbol, 42, at)
			_, ok := tape.symbols.Load(testSymbol)

			Convey("Lane exists with cfg bucket count", func() {
				So(ok, ShouldBeTrue)
				lane := tape.lane(testSymbol)
				So(len(lane.buckets), ShouldEqual, buckets)
				So(len(lane.bucketMinute), ShouldEqual, buckets)
			})
		})
	})
}

func BenchmarkNewTape(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewTape()
	}
}

func BenchmarkNote(b *testing.B) {
	tape := NewTape()
	at := time.Unix(1_700_000_000, 0).UTC()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		tape.Note("BTC/USD", 250.0, at)
	}
}

func BenchmarkUptake(b *testing.B) {
	tape := NewTape()
	now := time.Unix(1_700_000_600, 0).UTC()
	tape.Note("BTC/USD", 40.0, now)
	tape.Note("BTC/USD", 60.0, now.Add(-5*time.Minute))

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = tape.Uptake("BTC/USD", now)
	}
}

func BenchmarkLane(b *testing.B) {
	tape := NewTape()
	at := time.Unix(10_000, 0)
	tape.Note(testSymbol, 42, at)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = tape.lane(testSymbol)
	}
}
