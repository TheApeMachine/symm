package statistic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestCountNext(t *testing.T) {
	tests.Check(
		t, tests.Case[float64, float64]{
			Name: "count",
			Seed: 0.0,
			Factory: func() core.Primitive {
				return NewCount()
			},
			Reference: func(cnt, _ float64) float64 {
				return cnt + 1
			},
		},
	)

	Convey("Given a Count primitive", t, func() {
		op := NewCount()

		Convey("When a nil pointer arrives, it records ErrShape", func() {
			nilSeq := func(yield func(unsafe.Pointer) bool) {
				yield(nil)
			}
			out := tests.CollectSeq[float64](op.Next(nilSeq))

			So(len(out), ShouldEqual, 0)
			So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
		})
	})
}
