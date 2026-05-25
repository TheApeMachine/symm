package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewInverse(t *testing.T) {
	t.Parallel()

	Convey("Given NewInverse", t, func() {
		inv := NewInverse()

		Convey("It should start unobserved", func() {
			So(inv.observed, ShouldBeFalse)
		})
	})
}

func TestInverseNext(t *testing.T) {
	t.Parallel()

	Convey("Given an Inverse", t, func() {
		inv := NewInverse()

		Convey("It should error when no values are supplied", func() {
			_, err := inv.Next(0)

			So(err, ShouldNotBeNil)
		})

		Convey("It should echo the first observation while learning range", func() {
			v, err := inv.Next(0, 4)

			So(err, ShouldBeNil)
			So(v, ShouldEqual, 4)
		})

		Convey("It should mirror inside the observed min/max window", func() {
			_, _ = inv.Next(0, 0)
			_, _ = inv.Next(0, 10)
			v, err := inv.Next(0, 8)

			So(err, ShouldBeNil)
			So(v, ShouldAlmostEqual, 2.0, 1e-9)
		})
	})
}

func TestInverseReset(t *testing.T) {
	t.Parallel()

	Convey("Given an Inverse after learning range", t, func() {
		inv := NewInverse()

		_, _ = inv.Next(0, 1)
		_, _ = inv.Next(0, 9)

		So(inv.Reset(), ShouldBeNil)

		v, err := inv.Next(0, 5)

		So(err, ShouldBeNil)
		So(v, ShouldEqual, 5)
	})
}

func BenchmarkInverseNext(b *testing.B) {
	inv := NewInverse()

	var v float64

	var err error

	for idx := 0; idx < b.N; idx++ {
		v, err = inv.Next(v, float64(idx%9))

		if err != nil {
			b.Fatal(err)
		}
	}

	_ = v
}
