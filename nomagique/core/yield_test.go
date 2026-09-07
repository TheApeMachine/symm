package core_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestYield(t *testing.T) {
	Convey("Given a typed fold and a failure owner", t, func() {
		owner := transport.NewDiscard()
		seed := transport.NewIO(core.From(0.0))
		fold := func(total, value float64) float64 { return total + value }
		Convey("Successful runs still produce independent results and close normally", func() {
			input := transport.NewIO(core.From(2.0), core.From(3.0))
			result := core.Yield(seed, input, fold, owner)
			So(core.To[float64](result), ShouldEqual, 5)
			So(core.Yield(seed, input, fold, owner), ShouldBeNil)
			So(owner.Error(), ShouldBeNil)
			So(core.To[float64](result), ShouldEqual, 5)
		})
		Convey("A terminal source failure reaches both result and owner", func() {
			marker := errors.New("terminal failure")
			var source *transport.Generator[float64]
			source = transport.NewGenerator(func(yield func(float64) bool) {
				yield(2)
				source.Error(marker)
			})
			result := core.Yield(seed, source, fold, owner)
			So(errors.Is(result.Error(), marker), ShouldBeTrue)
			So(errors.Is(owner.Error(), marker), ShouldBeTrue)
		})
	})
}

func BenchmarkYield(b *testing.B) {
	seed := transport.NewIO(core.From(0.0))
	input := transport.NewIO(core.From(2.0), core.From(3.0))
	owner := transport.NewDiscard()
	fold := func(total, value float64) float64 { return total + value }
	b.ReportAllocs()
	for b.Loop() {
		if core.Yield(seed, input, fold, owner) == nil || core.Yield(seed, input, fold, owner) != nil {
			b.Fatal("fold delivery changed")
		}
	}
}
