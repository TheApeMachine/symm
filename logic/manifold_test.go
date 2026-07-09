package logic

import (
	"testing"
	"time"

	"github.com/theapemachine/nomagique/adaptive"

	. "github.com/smartystreets/goconvey/convey"
)

func TestManifoldNormalize(testingTB *testing.T) {
	Convey("Given a manifold metric baseline", testingTB, func() {
		manifold := &Manifold{
			halflife:  time.Second,
			baselines: map[string]*adaptive.TimeElastic{},
		}

		Convey("When equal values arrive after the baseline is ready", func() {
			_, ready := manifold.normalize("spread", 10, time.Unix(1, 0))
			So(ready, ShouldBeFalse)

			value, ready := manifold.normalize("spread", 10, time.Unix(2, 0))

			Convey("Then the normalized value is centered on zero deviation", func() {
				So(ready, ShouldBeTrue)
				So(value, ShouldAlmostEqual, 0, 0.000001)
			})
		})

		Convey("When a materially larger value arrives", func() {
			_, _ = manifold.normalize("volume", 10, time.Unix(1, 0))
			value, ready := manifold.normalize("volume", 20, time.Unix(2, 0))

			Convey("Then the normalized value is positive deviation", func() {
				So(ready, ShouldBeTrue)
				So(value, ShouldBeGreaterThan, 0)
			})
		})
	})
}
