package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestTeeNext(t *testing.T) {
	Convey("Tee yields arrivals downstream while branching to the offramp", t, func() {
		offramp := &counter{PrimitiveError: core.NewPrimitiveError()}
		tee := transport.NewTee(offramp)
		pipeline := nomagique.NewNumber(
			tee,
		)

		outputs := make([]float64, 0, 3)

		for out := range pipeline.Next(sequence.NewValue(10.0, 20.0, 30.0)) {
			outputs = append(outputs, *(*float64)(out))
		}

		So(pipeline.Error(), ShouldBeNil)
		So(outputs, ShouldResemble, []float64{10.0, 20.0, 30.0})
		So(offramp.count, ShouldEqual, 3.0)
	})

	Convey("Tee with nil offramp passes stream through unchanged", t, func() {
		tee := transport.NewTee(nil)
		outputs := make([]float64, 0, 2)

		for out := range tee.Next(sequence.NewValue(5.0, 15.0)) {
			outputs = append(outputs, *(*float64)(out))
		}

		So(tee.Error(), ShouldBeNil)
		So(outputs, ShouldResemble, []float64{5.0, 15.0})
	})
}
