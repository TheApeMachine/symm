package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAscentAbsorb(t *testing.T) {
	Convey("Setup", t, func() {
		ascent := &Ascent{}

		Convey("When a path reading is absorbed", func() {
			ascent.absorb(pathReading{
				meanUp:    0.2,
				meanDown:  0.5,
				recovered: 0.4,
				peak:      2.0,
				last:      1.6,
				samples:   6,
				cusumPos:  1.0,
				cusumNeg:  0.5,
				momentum:  0.1,
			})

			Convey("Wire fields mirror path memory", func() {
				So(ascent.MeanUp, ShouldEqual, 0.2)
				So(ascent.MeanDown, ShouldEqual, 0.5)
				So(ascent.RecoveredPullback, ShouldEqual, 0.4)
				So(ascent.NormalDown(), ShouldEqual, 0.5)
				So(ascent.Exhaust(), ShouldEqual, 0.5/1.5)
				So(ascent.Samples, ShouldEqual, 6)
			})
		})
	})
}

func TestAscentReset(t *testing.T) {
	Convey("Setup", t, func() {
		ascent := &Ascent{MeanUp: 1, MeanDown: 2, Samples: 4}
		ascent.Reset()

		Convey("Snapshot clears", func() {
			So(ascent.Samples, ShouldEqual, 0)
			So(ascent.MeanUp, ShouldEqual, 0)
			So(ascent.MeanDown, ShouldEqual, 0)
		})
	})
}
