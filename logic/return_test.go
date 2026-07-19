package logic

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestReturnHeadPredict proves strict-prior direction learning and rejects the
former behavior where opposite return processes received the same sign.
*/
func TestReturnHeadPredict(t *testing.T) {
	withResonanceRLS(t)

	Convey("Given alternating physical states with known next-return direction", t, func() {
		head, err := newReturnHead()
		So(err, ShouldBeNil)
		midPrice := 100.0
		priorDirection := 0.0
		prediction := 0.0

		for index := range 96 {
			if index > 0 {
				midPrice *= math.Exp(0.001 * priorDirection)
			}

			So(head.Resolve(midPrice), ShouldBeNil)
			direction := 1.0

			if index%2 == 1 {
				direction = -1
			}

			prediction, err = head.Predict(
				[]float64{direction, 0, 0, 0, 0}, midPrice,
			)
			So(err, ShouldBeNil)
			priorDirection = direction
		}

		Convey("Then the current prediction follows its row without future leakage", func() {
			So(prediction, ShouldBeLessThan, 0)
			So(head.Ready(), ShouldBeTrue)
			So(head.samples, ShouldEqual, uint64(95))
			So(head.meanMSE, ShouldBeGreaterThanOrEqualTo, 0)
			So(head.skillLower, ShouldBeGreaterThan, 0)
		})
	})
}

/*
BenchmarkReturnHeadPredict measures one real resolve-and-predict calibration
step with the configured RLS learner and confidence bound.
*/
func BenchmarkReturnHeadPredict(b *testing.B) {
	withResonanceRLS(b)
	head, err := newReturnHead()

	if err != nil {
		b.Fatal(err)
	}

	midPrice := 100.0
	features := []float64{1, 0, 0, 0, 0}

	if _, err := head.Predict(features, midPrice); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		midPrice *= math.Exp(0.0001)

		if err := head.Resolve(midPrice); err != nil {
			b.Fatal(err)
		}

		if _, err := head.Predict(features, midPrice); err != nil {
			b.Fatal(err)
		}
	}
}
