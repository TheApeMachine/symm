package learning_test

import (
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
)

func TestPaceNext(t *testing.T) {
	Convey("Pace stays at rest until the window is full, then moves in log space", t, func() {
		node := learning.NewPace(0.03, 0.005, 0.15, 0.1, 0.2, 8)
		rng := rand.New(rand.NewSource(86))

		for index := 0; index < 200; index++ {
			got := data.Read[[4]float64](node.Next(data.NewValue(rng.Float64() + float64(index/50))))

			So(node.Error(), ShouldBeNil)

			if index < 8 {
				So(got[2], ShouldEqual, 0)
				So(got[0], ShouldEqual, 0.03)
				So(got[1], ShouldEqual, 0)
			}

			if index >= 8 {
				So(got[2], ShouldEqual, 1)
				So(got[0], ShouldBeGreaterThan, 0)
				So(got[0], ShouldBeLessThanOrEqualTo, 0.15)
			}
		}
	})

	Convey("A rejected configuration yields nothing and records its error", t, func() {
		node := learning.NewPace(0.03, 0, 0.15, 0.1, 0.2, 8)
		So(node.Error(), ShouldNotBeNil)

		for range node.Next(data.NewValue(1.0)) {
			t.Fatal("rejected pace must yield nothing")
		}
	})
}
