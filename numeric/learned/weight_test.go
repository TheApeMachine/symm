package learned

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewWeight(t *testing.T) {
	t.Parallel()

	Convey("NewWeight returns a usable Weight", t, func() {
		So(NewWeight(0.35), ShouldNotBeNil)
	})
}

func TestWeightNext(t *testing.T) {
	t.Parallel()

	Convey("Given a Weight", t, func() {
		Convey("when fewer than two values are passed", func() {
			Convey("It should return stored value without changing", func() {
				weight := NewWeight(0.35)
				val, err := weight.Next(0, 0.5)

				So(err, ShouldBeNil)
				So(val, ShouldEqual, 0)
			})
		})

		Convey("when predictions match observations", func() {
			Convey("It should stay in [0,1]", func() {
				weight := NewWeight(0.35)

				for range 20 {
					_, err := weight.Next(0, 1, 1, 1)

					So(err, ShouldBeNil)
				}

				val, err := weight.Next(0, 3, 1, 1)

				So(err, ShouldBeNil)
				So(val, ShouldBeGreaterThanOrEqualTo, 0)
				So(val, ShouldBeLessThanOrEqualTo, 1)
			})
		})

		Convey("when receiver is nil", func() {
			Convey("It should error", func() {
				var weight *Weight
				_, err := weight.Next(0, 0, 1, 1)

				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestWeightReset(t *testing.T) {
	t.Parallel()

	Convey("Reset clears internal state", t, func() {
		weight := NewWeight(0.35)

		_, _ = weight.Next(0, 1, 0, 1)
		So(weight.Reset(), ShouldBeNil)

		val, err := weight.Next(0, 0.5)

		So(err, ShouldBeNil)
		So(val, ShouldEqual, 0)
	})

	Convey("Reset on nil errors", t, func() {
		var weight *Weight

		So(weight.Reset(), ShouldNotBeNil)
	})
}

func BenchmarkWeightNext(b *testing.B) {
	weight := NewWeight(0.35)

	b.ResetTimer()

	for b.Loop() {
		_, _ = weight.Next(0, 1.0, 0.8)
	}
}
