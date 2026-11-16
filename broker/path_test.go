package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPathNote(t *testing.T) {
	Convey("Setup", t, func() {
		path := newPath()
		t0 := time.Unix(100, 0)

		Convey("When crawl chops then recovers", func() {
			steps := []float64{1.02, 1.03, 1.04, 1.01, 1.05, 1.06, 1.03, 1.07}
			var last pathReading

			for i, pnl := range steps {
				last = path.note(pnl, t0.Add(time.Duration(i)*100*time.Millisecond))
			}

			Convey("Down memory learns without arming an exit on familiar dips", func() {
				So(last.meanDown, ShouldBeGreaterThan, 0)
				So(last.recovered, ShouldBeGreaterThan, 0)
				So(last.armedExit, ShouldBeFalse)
				So(last.gateArmed, ShouldBeFalse)
			})
		})

		Convey("When only ups have printed", func() {
			path.note(1.1, t0)
			path.note(1.3, t0.Add(100*time.Millisecond))
			path.note(1.6, t0.Add(200*time.Millisecond))
			reading := path.note(1.55, t0.Add(300*time.Millisecond))

			Convey("Ignition shape — no down baseline yet", func() {
				So(reading.downReady, ShouldBeFalse)
				So(reading.meanUp, ShouldBeGreaterThan, 0)
				So(reading.ignition, ShouldBeTrue)
			})
		})

		Convey("When unchanged pnl repeats", func() {
			path.note(1.2, t0)
			path.note(1.4, t0.Add(time.Second))
			before := path.samples
			path.note(1.4, t0.Add(2*time.Second))

			Convey("Samples do not advance", func() {
				So(path.samples, ShouldEqual, before)
			})
		})
	})
}

func TestPathReset(t *testing.T) {
	Convey("Setup", t, func() {
		path := newPath()
		path.note(1.0, time.Unix(1, 0))
		path.note(1.5, time.Unix(2, 0))
		path.reset()

		Convey("Memory is cold again", func() {
			So(path.samples, ShouldEqual, 0)
			So(path.hasLast, ShouldBeFalse)
		})
	})
}
