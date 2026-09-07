package core_test

import (
	"errors"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"strings"
	"testing"

	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPrimitiveErrorError(t *testing.T) {
	Convey("Given branches propagating overlapping failures back to their owners", t, func() {
		first := fmt.Errorf("left operand: %w", core.ErrShape)
		second := fmt.Errorf("right operand: %w", core.ErrNotHeld)
		var left, right core.PrimitiveError
		left.Error(first)
		right.Error(second)

		Convey("Repeated propagation preserves each context once and keeps nil reads inert", func() {
			for range 12 {
				left.Error(right.Error())
				right.Error(left.Error())
			}
			So(errors.Is(left.Error(), core.ErrShape), ShouldBeTrue)
			So(errors.Is(left.Error(), core.ErrNotHeld), ShouldBeTrue)
			So(strings.Count(left.Error().Error(), "left operand"), ShouldEqual, 1)
			So(strings.Count(left.Error().Error(), "right operand"), ShouldEqual, 1)
			So(left.Error(nil), ShouldEqual, left.Error())
		})
	})
}

// Type failures must survive both the returned value and the run boundary.
func TestErrorTravels(t *testing.T) {
	tests.CheckTypeFailure(t, arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))))
}

func BenchmarkPrimitiveErrorError(b *testing.B) {
	var left, right core.PrimitiveError
	left.Error(fmt.Errorf("left operand: %w", core.ErrShape))
	right.Error(fmt.Errorf("right operand: %w", core.ErrNotHeld))
	left.Error(right.Error())
	right.Error(left.Error())
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		left.Error(right.Error())
		right.Error(left.Error())
	}
}
