package learning_test

import (
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPaceNext(t *testing.T) {
	Convey("Pace stays at rest until the window is full, then moves in log space", t, func() {
		node := learning.NewPace(learning.PaceConfig{
			Rest: 0.03, Lower: 0.005, Upper: 0.15, Gain: 0.1, Band: 0.2, Window: 8,
		})
		rng := rand.New(rand.NewSource(86))

		for index := 0; index < 200; index++ {
			got, err := transport.Evaluate(node, transport.Values(rng.Float64()+float64(index/50)))
			So(err, ShouldBeNil)

			if index < 8 {
				So(got.Ready, ShouldBeFalse)
				So(got.Alpha, ShouldEqual, 0.03)
				So(got.Rank, ShouldEqual, 0)
			}

			if index >= 8 {
				So(got.Ready, ShouldBeTrue)
				So(got.Alpha, ShouldBeGreaterThan, 0)
				So(got.Alpha, ShouldBeLessThanOrEqualTo, 0.15)
			}
		}
	})
}
