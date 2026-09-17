package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestGateNext(t *testing.T) {
	Convey("Gate delivers the original pair to the selected arithmetic branch", t, func() {
		gate := NewGate(NewGreater(), arithmetic.NewAdd(), arithmetic.NewSubtract())
		input := sequence.NewValue([2]float64{5, 2}, [2]float64{1, 3}, [2]float64{2, 2})
		So(tests.CollectSeq[float64](gate.Next(input)), ShouldResemble, []float64{7, -2, 0})
		So(gate.Error(), ShouldBeNil)
	})
}
