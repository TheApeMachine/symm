package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewAccumulator(t *testing.T) {
	t.Parallel()

	Convey("Given NewAccumulator with a non-zero starting level", t, func() {
		acc := NewAccumulator(12.5)

		Convey("It should keep the seed as the baseline before updates", func() {
			level, err := acc.Next(0, 0)

			So(err, ShouldBeNil)
			So(level, ShouldEqual, 12.5)
		})
	})
}

func TestAccumulatorNext(t *testing.T) {
	t.Parallel()

	Convey("Given an Accumulator", t, func() {
		acc := NewAccumulator(0)

		Convey("It should integrate smoothed signed observations", func() {
			level, err := acc.Next(0, 4, -1, 2)

			So(err, ShouldBeNil)
			So(level, ShouldBeGreaterThan, 4.0)
		})
	})
}

func TestAccumulatorReset(t *testing.T) {
	t.Parallel()

	Convey("Given an Accumulator after updates", t, func() {
		acc := NewAccumulator(0)

		_, err := acc.Next(0, 10)

		So(err, ShouldBeNil)

		Convey("Reset should zero the integral and smoother", func() {
			So(acc.Reset(), ShouldBeNil)

			level, err := acc.Next(0, 0)

			So(err, ShouldBeNil)
			So(level, ShouldEqual, 0)
		})
	})
}

func BenchmarkAccumulatorNext(b *testing.B) {
	acc := NewAccumulator(0)

	var level float64

	var err error

	for idx := 0; idx < b.N; idx++ {
		level, err = acc.Next(0, float64(idx%13)*0.07-0.2)

		if err != nil {
			b.Fatal(err)
		}
	}

	_ = level
}
