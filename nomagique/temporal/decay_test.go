package temporal

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestDecayNext(t *testing.T) {
	Convey("A missing clock extinguishes a finite input", t, func() {
		out := tests.CollectSeq(NewDecay[float64](nil, nil).Next(transport.Values(10.0)))
		So(out[0], ShouldEqual, 0)
	})

	Convey("Linear retention uses one minus elapsed, floored at zero", t, func() {
		out := tests.CollectSeq(
			NewDecay[float64](store.NewConstant[float64, float64](0.25), nil).Next(transport.Values(10.0)),
		)
		So(out[0], ShouldEqual, 7.5)
	})

	Convey("A configured shape sees elapsed time, not a second protocol", t, func() {
		shape := transport.NewStages(
			calculus.NewNegate[float64](),
			calculus.NewExp[float64](),
		)
		out := tests.CollectSeq(
			NewDecay[float64](store.NewConstant[float64, float64](math.Ln2), shape).Next(transport.Values(10.0)),
		)
		So(out[0], ShouldEqual, 5)
	})
}
