package temporal_test

import (
	"math"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestDecayNext(t *testing.T) {
	Convey("A missing clock extinguishes a finite input", t, func() {
		val := 10.0
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&val))
		}
		out := tests.CollectSeq[float64](temporal.NewDecay(nil, nil).Next(in))
		So(out[0], ShouldEqual, 0)
	})

	Convey("Linear retention uses one minus elapsed, floored at zero", t, func() {
		val := 10.0
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&val))
		}
		node := temporal.NewDecay(store.NewConstant(0.25), nil)
		out := tests.CollectSeq[float64](node.Next(in))
		So(out[0], ShouldEqual, 7.5)
	})

	Convey("A configured shape sees elapsed time, not a second protocol", t, func() {
		shape := nomagique.NewNumber(
			calculus.NewNegate(),
			calculus.NewExp(),
		)
		val := 10.0
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&val))
		}
		node := temporal.NewDecay(store.NewConstant(math.Ln2), shape)
		out := tests.CollectSeq[float64](node.Next(in))
		So(out[0], ShouldEqual, 5)
	})
}
