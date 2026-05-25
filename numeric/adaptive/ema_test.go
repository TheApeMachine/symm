package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewEMA(t *testing.T) {
	t.Parallel()

	Convey("Given a fresh EMA", t, func() {
		ema := NewEMA(0.35)

		Convey("It should not be observed until the first sample", func() {
			So(ema.observed, ShouldBeFalse)
		})
	})
}

func TestEMANext(t *testing.T) {
	t.Parallel()

	Convey("Given an EMA fed a staircase", t, func() {
		ema := NewEMA(0.35)

		_, err := ema.Next(0, 1.0)

		So(err, ShouldBeNil)

		smoothed, err := ema.Next(0, 1.0, 1.0, 1.0)

		So(err, ShouldBeNil)
		So(smoothed, ShouldAlmostEqual, 1.0, 1e-9)
	})

	Convey("Given an EMA after a multi-input pipeline stage", t, func() {
		ema := NewEMA(0.35)

		smoothed, err := ema.Next(0.24, 0.8, 0.3)

		So(err, ShouldBeNil)
		So(smoothed, ShouldAlmostEqual, 0.24, 1e-9)
		So(ema.Value(), ShouldAlmostEqual, 0.24, 1e-9)
	})

	Convey("Given an EMA when the prior stage gated to zero", t, func() {
		ema := NewEMA(0.35)

		_, _ = ema.Next(0, 5)

		out, err := ema.Next(0, 0.8, 0.3)

		So(err, ShouldBeNil)
		So(out, ShouldEqual, 0)
		So(ema.Value(), ShouldEqual, 5)
	})
}

func TestEMAReset(t *testing.T) {
	t.Parallel()

	Convey("Given an EMA after observations", t, func() {
		ema := NewEMA(0.35)

		_, _ = ema.Next(0, 42)

		So(ema.Reset(), ShouldBeNil)

		v, err := ema.Next(0, 7)

		So(err, ShouldBeNil)
		So(v, ShouldEqual, 7)
	})
}

func TestEMAClone(t *testing.T) {
	t.Parallel()

	Convey("Given a cloned EMA", t, func() {
		orig := NewEMA(0.35)

		_, _ = orig.Next(0, 5)
		_, _ = orig.Next(0, 10)

		snapshot := orig.value

		copied := orig.Clone()

		Convey("Clone should preserve internal state", func() {
			So(copied.value, ShouldEqual, snapshot)
			So(copied.observed, ShouldBeTrue)
		})

		Convey("Mutating the clone should not move the original", func() {
			_, _ = copied.Next(0, 999)

			So(orig.value, ShouldEqual, snapshot)
			So(copied.value, ShouldNotEqual, snapshot)
		})
	})

	Convey("Clone on nil receiver returns nil", t, func() {
		var nilEMA *EMA

		So(nilEMA.Clone(), ShouldBeNil)
	})
}

func BenchmarkEMANext(b *testing.B) {
	ema := NewEMA(0.35)

	var v float64

	var err error

	for idx := 0; idx < b.N; idx++ {
		v, err = ema.Next(v, float64(idx%11))

		if err != nil {
			b.Fatal(err)
		}
	}

	_ = v
}
