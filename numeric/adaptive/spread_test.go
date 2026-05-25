package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewSpread(t *testing.T) {
	t.Parallel()

	Convey("Given NewSpread", t, func() {
		spread := NewSpread(0.35)

		out, err := spread.Next(0, 1)

		So(err, ShouldBeNil)
		So(out, ShouldEqual, 1)
	})
}

func TestSpreadNext(t *testing.T) {
	t.Parallel()

	Convey("Given a Spread with zero reference out", t, func() {
		spread := NewSpread(0.35)

		Convey("It should square the observation as the first EMA sample", func() {
			out, err := spread.Next(0, 7)

			So(err, ShouldBeNil)
			So(out, ShouldEqual, 49)
		})
	})
}

func TestSpreadReset(t *testing.T) {
	t.Parallel()

	Convey("Given a Spread after observations", t, func() {
		spread := NewSpread(0.35)

		_, err := spread.Next(0, 4)

		So(err, ShouldBeNil)

		So(spread.Reset(), ShouldBeNil)

		again, err := spread.Next(0, 4)

		So(err, ShouldBeNil)
		So(again, ShouldEqual, 16)
	})
}

func BenchmarkSpreadNext(b *testing.B) {
	spread := NewSpread(0.35)

	var v float64

	var err error

	for idx := 0; idx < b.N; idx++ {
		v, err = spread.Next(v, float64(idx%9))

		if err != nil {
			b.Fatal(err)
		}
	}

	_ = v
}
