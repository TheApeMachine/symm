package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAscentNote(t *testing.T) {
	Convey("Setup", t, func() {
		ascent := &Ascent{}
		t0 := time.Unix(100, 0)

		Convey("When the first reading lands", func() {
			ascent.Note(1.0, t0)

			Convey("Samples is one and velocity is untouched", func() {
				So(ascent.Samples, ShouldEqual, 1)
				So(ascent.LastPnLPct, ShouldEqual, 1.0)
				So(ascent.SmoothVelocity, ShouldEqual, 0)
				So(ascent.PeakVelocity, ShouldEqual, 0)
			})
		})

		Convey("When a second reading climbs over 100ms", func() {
			ascent.Note(1.0, t0)
			t1 := t0.Add(100 * time.Millisecond)
			ascent.Note(1.5, t1)

			Convey("EMA velocity is 2.5 per second", func() {
				So(ascent.Samples, ShouldEqual, 2)
				So(ascent.SmoothVelocity, ShouldEqual, 2.5)
				So(ascent.PeakVelocity, ShouldEqual, 2.5)
			})
		})

		Convey("When a later timestamp is before the last", func() {
			ascent.Note(2.0, t0)
			ascent.Note(1.0, t0.Add(-time.Second))

			Convey("The tracker restarts at the new reading", func() {
				So(ascent.Samples, ShouldEqual, 1)
				So(ascent.LastPnLPct, ShouldEqual, 1.0)
			})
		})
	})
}

func TestAscentExhaust(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When peak velocity was never established", func() {
			ascent := &Ascent{}

			Convey("Exhaust is zero", func() {
				So(ascent.Exhaust(), ShouldEqual, 0)
			})
		})

		Convey("When smoothed velocity still matches peak", func() {
			ascent := &Ascent{PeakVelocity: 2.5, SmoothVelocity: 2.5}

			Convey("Exhaust is zero", func() {
				So(ascent.Exhaust(), ShouldEqual, 0)
			})
		})

		Convey("When climb velocity halves against peak", func() {
			ascent := &Ascent{PeakVelocity: 2.5, SmoothVelocity: 1.25}

			Convey("Exhaust is 0.5", func() {
				So(ascent.Exhaust(), ShouldEqual, 0.5)
			})
		})

		Convey("When smoothed velocity is zero", func() {
			ascent := &Ascent{PeakVelocity: 2.5, SmoothVelocity: 0}

			Convey("Exhaust is 1.0", func() {
				So(ascent.Exhaust(), ShouldEqual, 1.0)
			})
		})

		Convey("When two flat ticks follow a climb", func() {
			ascent := &Ascent{}
			t0 := time.Unix(100, 0)
			ascent.Note(1.0, t0)
			t1 := t0.Add(100 * time.Millisecond)
			ascent.Note(1.5, t1)
			t2 := t1.Add(100 * time.Millisecond)
			ascent.Note(1.5, t2)
			t3 := t2.Add(100 * time.Millisecond)
			ascent.Note(1.5, t3)

			Convey("Exhaust reaches 0.75", func() {
				So(ascent.Exhaust(), ShouldEqual, 0.75)
			})
		})
	})
}

func TestAscentReset(t *testing.T) {
	Convey("Setup", t, func() {
		ascent := &Ascent{}
		t0 := time.Unix(100, 0)
		ascent.Note(1.0, t0)
		ascent.Note(2.0, t0.Add(time.Second))

		Convey("When Reset is called", func() {
			ascent.Reset()

			Convey("All kinematic fields zero out", func() {
				So(ascent.Samples, ShouldEqual, 0)
				So(ascent.PeakVelocity, ShouldEqual, 0)
				So(ascent.SmoothVelocity, ShouldEqual, 0)
				So(ascent.LastPnLPct, ShouldEqual, 0)
				So(ascent.LastTime.IsZero(), ShouldBeTrue)
			})
		})
	})
}

func BenchmarkAscentNote(b *testing.B) {
	ascent := &Ascent{}
	t0 := time.Unix(100, 0)
	t1 := t0.Add(100 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ascent.Reset()
		ascent.Note(1.0, t0)
		ascent.Note(1.5, t1)
	}
}

func BenchmarkAscentExhaust(b *testing.B) {
	ascent := &Ascent{PeakVelocity: 2.5, SmoothVelocity: 1.25}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = ascent.Exhaust()
	}
}

func BenchmarkAscentReset(b *testing.B) {
	ascent := &Ascent{}
	t0 := time.Unix(100, 0)
	ascent.Note(1.0, t0)
	ascent.Note(2.0, t0.Add(time.Second))

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		ascent.Reset()
		ascent.Note(1.0, t0)
		ascent.Note(2.0, t0.Add(time.Second))
	}
}
