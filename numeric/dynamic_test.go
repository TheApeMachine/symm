package numeric

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/adaptive"
)

type stubDynamicErr struct{}

func (stubDynamicErr) Next(float64, ...float64) (float64, error) {
	return 0, errors.New("stub dynamic failure")
}

func (stubDynamicErr) Reset() error {
	return nil
}

func TestNewDerived(t *testing.T) {
	t.Parallel()

	Convey("Given NewDerived with WithDynamics", t, func() {
		chain := NewDerived(WithDynamics(adaptive.NewEMA(0.35)))

		So(chain, ShouldNotBeNil)
		So(len(chain.dynamics), ShouldEqual, 1)
	})
}

func TestDerivedNext(t *testing.T) {
	t.Parallel()

	Convey("Given a Derived EMA chain", t, func() {
		chain := NewDerived(WithDynamics(adaptive.NewEMA(0.35)))

		out, err := chain.Push(2.0, 2.0, 2.0)

		So(err, ShouldBeNil)
		So(out, ShouldAlmostEqual, 2.0, 1e-9)
		So(chain.Value(), ShouldAlmostEqual, out, 1e-9)
	})

	Convey("Given a Derived whose first dynamic errors", t, func() {
		chain := NewDerived(WithDynamics(stubDynamicErr{}))

		_, err := chain.Push(1)

		So(err, ShouldNotBeNil)
	})
}

func TestDerivedReset(t *testing.T) {
	t.Parallel()

	Convey("Given a Derived after Next", t, func() {
		chain := NewDerived(WithDynamics(adaptive.NewEMA(0.35)))

		_, _ = chain.Push(9)

		So(chain.Reset(), ShouldBeNil)

		fresh, err := chain.Push(3)

		So(err, ShouldBeNil)
		So(fresh, ShouldEqual, 3)
	})
}

func TestDerivedValue(t *testing.T) {
	t.Parallel()

	Convey("Given NewDerivedFrom without dynamics", t, func() {
		chain := NewDerivedFrom(42)

		So(chain.Value(), ShouldEqual, 42)

		_, err := chain.Push(100)

		So(err, ShouldBeNil)
		So(chain.Value(), ShouldEqual, 0)
	})
}

func TestDerivedClone(t *testing.T) {
	t.Parallel()

	Convey("Given a Derived clone of an EMA-backed chain", t, func() {
		original := NewDerived(WithDynamics(adaptive.NewEMA(0.35)))

		_, _ = original.Push(4, 8)

		snapshot := original.Value()

		clone := original.Clone()

		So(clone, ShouldNotBeNil)
		So(clone.Value(), ShouldEqual, snapshot)

		_, _ = clone.Push(1000)

		So(original.Value(), ShouldEqual, snapshot)
		So(clone.Value(), ShouldNotEqual, snapshot)
	})

	Convey("Clone on nil Derived returns nil", t, func() {
		var nilDerived *Derived

		So(nilDerived.Clone(), ShouldBeNil)
	})
}

func TestWithDynamics(t *testing.T) {
	t.Parallel()

	Convey("WithDynamics appends multiple dynamics in order", t, func() {
		first := adaptive.NewEMA(0.35)
		second := adaptive.NewSpread(0.35)

		chain := NewDerived(WithDynamics(first, second))

		So(len(chain.dynamics), ShouldEqual, 2)
		So(chain.dynamics[0], ShouldEqual, first)
		So(chain.dynamics[1], ShouldEqual, second)
	})

	Convey("Product then EMA smooths the product not the raw inputs", t, func() {
		chain := NewDerived(WithDynamics(adaptive.NewProduct(), adaptive.NewEMA(0)))

		out, err := chain.Push(0.8, 0.3)

		So(err, ShouldBeNil)
		So(out, ShouldAlmostEqual, 0.24, 1e-9)
	})

	Convey("EMA then Compression scores spread tightening", t, func() {
		chain := NewDerived(WithDynamics(adaptive.NewEMA(0), adaptive.NewCompression(0)))

		for range 8 {
			_, _ = chain.Push(20)
		}

		out, err := chain.Push(10)

		So(err, ShouldBeNil)
		So(out, ShouldBeGreaterThan, 1)
	})
}

func BenchmarkDerivedNext(b *testing.B) {
	chain := NewDerived(WithDynamics(adaptive.NewEMA(0.35)))

	var v float64

	var err error

	for idx := 0; idx < b.N; idx++ {
		v, err = chain.Push(float64(idx%13) * 0.1)

		if err != nil {
			b.Fatal(err)
		}
	}

	_ = v
}

func BenchmarkDerivedClone(b *testing.B) {
	original := NewDerived(WithDynamics(adaptive.NewEMA(0.35), adaptive.NewSigmaClamp(2, 8, 0.1)))

	_, _ = original.Push(1.5, 2.5, 3.5)

	var c *Derived

	b.ResetTimer()

	for b.Loop() {
		c = original.Clone()
	}

	_ = c
}
