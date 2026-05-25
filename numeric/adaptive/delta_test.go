package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewDelta(t *testing.T) {
	t.Parallel()

	Convey("Given NewDelta anchored to the first observation", t, func() {
		delta := NewDelta(5)

		Convey("It should start with that anchor as the previous sample", func() {
			So(delta.previous, ShouldEqual, 5)
		})
	})
}

func TestDeltaNext(t *testing.T) {
	t.Parallel()

	Convey("Given a Delta aligned with the first sample", t, func() {
		delta := NewDelta(10)

		Convey("It should report no change when the observation repeats", func() {
			change, err := delta.Next(0, 10)

			So(err, ShouldBeNil)
			So(change, ShouldEqual, 0)
		})

		Convey("It should track subsequent motion", func() {
			first, err := delta.Next(0, 10)

			So(err, ShouldBeNil)
			So(first, ShouldEqual, 0)

			second, err := delta.Next(first, 20)

			So(err, ShouldBeNil)
			So(second, ShouldBeGreaterThan, 0)
		})
	})
}

func TestDeltaReset(t *testing.T) {
	t.Parallel()

	Convey("Given a Delta after movement", t, func() {
		delta := NewDelta(0)

		_, _ = delta.Next(0, 1)
		_, _ = delta.Next(0, 3)

		So(delta.Reset(), ShouldBeNil)

		change, err := delta.Next(0, 0)

		So(err, ShouldBeNil)
		So(change, ShouldEqual, 0)
	})
}

func BenchmarkDeltaNext(b *testing.B) {
	delta := NewDelta(0)

	var v float64

	var err error

	for idx := 0; idx < b.N; idx++ {
		v, err = delta.Next(v, float64(idx%5))

		if err != nil {
			b.Fatal(err)
		}
	}

	_ = v
}
