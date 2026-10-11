package adaptive_test

import (
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/transport"
)

type dummyWindow struct {
	out adaptive.WindowReading
}

func (d *dummyWindow) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			val := *(*float64)(arriving)
			d.out = adaptive.WindowReading{
				Value:     val,
				ShedRatio: 1.0,
				Capacity:  100.0,
			}
			if !yield(unsafe.Pointer(&d.out)) {
				return
			}
		}
	}
}

func (d *dummyWindow) Error(errs ...error) error {
	return nil
}

func TestBaseline_PriorVariance(t *testing.T) {
	Convey("Given a Baseline with a non-shedding window", t, func() {
		node := adaptive.NewBaseline(&dummyWindow{})

		Convey("Sequence [1, 3, 5] verifies causal prior variance progression", func() {
			// First arrival: 1.0
			v1 := 1.0
			var r1 adaptive.BaselineReading
			for out := range node.Next(transport.NewOne(unsafe.Pointer(&v1)).Next(nil)) {
				r1 = *(*adaptive.BaselineReading)(out)
			}
			So(r1.HasPrior, ShouldBeFalse)
			So(r1.Prior.Count, ShouldEqual, 0)
			So(r1.PriorVariance, ShouldEqual, 0)

			// Second arrival: 3.0
			v2 := 3.0
			var r2 adaptive.BaselineReading
			for out := range node.Next(transport.NewOne(unsafe.Pointer(&v2)).Next(nil)) {
				r2 = *(*adaptive.BaselineReading)(out)
			}
			So(r2.HasPrior, ShouldBeTrue)
			So(r2.Prior.Count, ShouldEqual, 1)
			// Prior count = 1 <= 1, insufficient prior support for variance
			So(r2.PriorVariance, ShouldEqual, 0)

			// Third arrival: 5.0
			// Prior samples: [1, 3]. Mean = 2, M2 = (1-2)^2 + (3-2)^2 = 2.
			// Prior sample variance = M2 / (Count - 1) = 2 / (2 - 1) = 2.
			v3 := 5.0
			var r3 adaptive.BaselineReading
			for out := range node.Next(transport.NewOne(unsafe.Pointer(&v3)).Next(nil)) {
				r3 = *(*adaptive.BaselineReading)(out)
			}
			So(r3.HasPrior, ShouldBeTrue)
			So(r3.Prior.Count, ShouldEqual, 2)
			So(r3.PriorVariance, ShouldAlmostEqual, 2.0, 1e-9)
		})

		Convey("Equal prior samples yield genuinely defined zero sample variance", func() {
			nodeEqual := adaptive.NewBaseline(&dummyWindow{})
			val := 42.0

			for range 2 {
				for out := range nodeEqual.Next(transport.NewOne(unsafe.Pointer(&val)).Next(nil)) {
					_ = *(*adaptive.BaselineReading)(out)
				}
			}

			// Third observation of equal value
			var r3 adaptive.BaselineReading
			for out := range nodeEqual.Next(transport.NewOne(unsafe.Pointer(&val)).Next(nil)) {
				r3 = *(*adaptive.BaselineReading)(out)
			}
			So(r3.Prior.Count, ShouldEqual, 2)
			So(r3.Prior.M2, ShouldAlmostEqual, 0.0, 1e-9)
			So(r3.PriorVariance, ShouldAlmostEqual, 0.0, 1e-9)
		})
	})
}
